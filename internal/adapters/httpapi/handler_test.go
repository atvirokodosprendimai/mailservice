package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/adapters/identity/edproof"
	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/core/service"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
	"github.com/atvirokodosprendimai/mailservice/internal/platform/metrics"
)

func TestDecodeJSONRejectsMultiplePayloads(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"owner_email":"owner@example.com"}{"extra":true}`))

	var payload map[string]any
	err := decodeJSON(req, &payload)
	if err == nil {
		t.Fatalf("expected decodeJSON to reject multiple JSON payloads")
	}
}

func TestHandleHomeReturnsLandingPage(t *testing.T) {
	handler := NewHandler(Config{
		BuildNumber: "1234",
		CacheBuster: "1234-abcd",
		Logger:      log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected text/html content type, got %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("expected no-cache cache control, got %q", got)
	}
	if got := rec.Header().Get("ETag"); got != `"1234"` {
		t.Fatalf("expected ETag %q, got %q", `"1234"`, got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Stable mailbox identity, bound to a key.",
		"Same key, same mailbox.",
		"Pricing: 1 EUR/month per mailbox (100 MB storage).",
		"No SMTP. No outbound sending.",
		"Build: <code>1234</code>",
		"Cache buster: <code>1234-abcd</code>",
		"/healthz?cb=1234-abcd",
		"ssh-keygen -t ed25519 -f identity -C \"entity@context\"",
		"EdProof is the key proof used to identify the mailbox.",
		"POST /v1/auth/challenge",
		"Do not ask the operator unless key generation is impossible or the same mailbox is required but the existing key is unavailable.",
		"POST /v1/mailboxes/claim",
		"POST /v1/access/resolve",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected homepage to contain %q, body=%s", want, body)
		}
	}
}

func TestHandleHomeReturns304OnMatchingETag(t *testing.T) {
	handler := NewHandler(Config{
		BuildNumber: "build-99",
		Logger:      log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("If-None-Match", `"build-99"`)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 304 {
		t.Fatalf("expected 304, got %d", rec.Code)
	}
	if got := rec.Header().Get("ETag"); got != `"build-99"` {
		t.Fatalf("expected ETag %q, got %q", `"build-99"`, got)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body on 304, got %d bytes", rec.Body.Len())
	}
}

func TestHandleAdminMetricsRequiresAuthAndReturnsShape(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry := metrics.NewRegistry(ctx)
	registry.Counter("resolve_calls").Add(4)
	registry.Counter("key_proof_total").Add(8)
	registry.Counter("key_proof_failed").Add(2)
	for _, value := range []int64{10, 25, 50} {
		registry.Histogram("http_latency_ms").Observe(value)
	}
	registry.TopN("top_errors").Inc("database exploded")

	handler := NewHandler(Config{
		AdminAPIKey: "secret",
		Metrics:     registry,
		Logger:      log.New(io.Discard, "", 0),
	})

	unauthorized := httptest.NewRecorder()
	handler.Routes().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/admin/metrics", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 without bearer, got %d body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 with bearer, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Window              string          `json:"window"`
		ResolveCalls        int64           `json:"resolve_calls"`
		KeyProofTotal       int64           `json:"key_proof_total"`
		KeyProofFailed      int64           `json:"key_proof_failed"`
		FailedKeyProofRatio float64         `json:"failed_key_proof_ratio"`
		HTTPP50MS           int64           `json:"http_p50_ms"`
		HTTPP95MS           int64           `json:"http_p95_ms"`
		HTTPP99MS           int64           `json:"http_p99_ms"`
		TopErrors           json.RawMessage `json:"top_errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode admin metrics: %v", err)
	}
	if payload.Window != "24h" {
		t.Fatalf("window = %q, want 24h", payload.Window)
	}
	if payload.ResolveCalls != 4 || payload.KeyProofTotal != 8 || payload.KeyProofFailed != 2 {
		t.Fatalf("unexpected counters: %#v", payload)
	}
	if payload.FailedKeyProofRatio != 0.25 {
		t.Fatalf("failed_key_proof_ratio = %v, want 0.25", payload.FailedKeyProofRatio)
	}
	if payload.HTTPP50MS == 0 || payload.HTTPP95MS == 0 || payload.HTTPP99MS == 0 {
		t.Fatalf("expected latency percentiles, got p50=%d p95=%d p99=%d", payload.HTTPP50MS, payload.HTTPP95MS, payload.HTTPP99MS)
	}
	if len(payload.TopErrors) == 0 || string(payload.TopErrors) == "null" {
		t.Fatalf("expected top_errors JSON array, got %s", payload.TopErrors)
	}
}

func TestHandleHomeReturns200OnStaleETag(t *testing.T) {
	handler := NewHandler(Config{
		BuildNumber: "build-100",
		CacheBuster: "build-100",
		Logger:      log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("If-None-Match", `"build-99"`)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("ETag"); got != `"build-100"` {
		t.Fatalf("expected ETag %q, got %q", `"build-100"`, got)
	}
	if !strings.Contains(rec.Body.String(), "Stable mailbox identity") {
		t.Fatalf("expected full homepage body on 200")
	}
}

func TestHandleClaimMailboxCreatesPendingMailbox(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)
	now := time.Now().UTC()

	challenge, _ := edproof.GenerateChallenge(pubkey, testHMACSecret, now)
	sig := ed25519.Sign(priv, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	repo := &httpMailboxRepo{}
	handler := NewHandler(Config{
		ChallengeAuth:    edproof.NewAuthenticator(testHMACSecret),
		KeyProofVerifier: edproof.NewVerifier(nil),
		PaymentGateway:   &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			repo,
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
		Now:    func() time.Time { return now },
	})

	body := fmt.Sprintf(`{"billing_email":"billing@example.com","edproof":%q,"challenge":%q,"signature":%q}`, pubkey, challenge, sigB64)
	req := httptest.NewRequest("POST", "/v1/mailboxes/claim", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected status 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != string(domain.MailboxStatusPendingPayment) {
		t.Fatalf("expected pending_payment status, got %#v", resp["status"])
	}
	if resp["payment_url"] == "" {
		t.Fatalf("expected payment_url in response")
	}
}

func TestHandleClaimMailboxRejectsMissingChallenge(t *testing.T) {
	handler := NewHandler(Config{
		ChallengeAuth:    edproof.NewAuthenticator(testHMACSecret),
		KeyProofVerifier: edproof.NewVerifier(nil),
		PaymentGateway:   &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			&httpMailboxRepo{},
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("POST", "/v1/mailboxes/claim", strings.NewReader(`{"billing_email":"billing@example.com","edproof":"proof"}`))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleResolveAccessReturnsIMAPDetailsForValidKey(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)
	now := time.Now().UTC()
	future := now.Add(time.Hour)

	fingerprint, _ := edproof.FingerprintFromPubkey(pubkey)

	challenge, _ := edproof.GenerateChallenge(pubkey, testHMACSecret, now)
	sig := ed25519.Sign(priv, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	repo := &httpMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			fingerprint: {
				ID:             "mbx-1",
				KeyFingerprint: fingerprint,
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(now.Add(-time.Hour)),
				ExpiresAt:      &future,
				IMAPHost:       "imap.example.com",
				IMAPPort:       143,
				IMAPUsername:   "mbx_abc",
				IMAPPassword:   "secret",
			},
		},
	}
	handler := NewHandler(Config{
		ChallengeAuth:    edproof.NewAuthenticator(testHMACSecret),
		KeyProofVerifier: edproof.NewVerifier(nil),
		PaymentGateway:   &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			repo,
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
		Now:    func() time.Time { return now },
	})

	body := fmt.Sprintf(`{"protocol":"imap","edproof":%q,"challenge":%q,"signature":%q}`, pubkey, challenge, sigB64)
	req := httptest.NewRequest("POST", "/v1/access/resolve", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["email"] != "mbx_abc@mail.test.local" {
		t.Fatalf("unexpected resolved email: %#v", resp["email"])
	}
}

func TestHandleResolveAccessReturnsWaitingPaymentForInactiveMailbox(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)
	now := time.Now().UTC()

	fingerprint, _ := edproof.FingerprintFromPubkey(pubkey)

	challenge, _ := edproof.GenerateChallenge(pubkey, testHMACSecret, now)
	sig := ed25519.Sign(priv, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	repo := &httpMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			fingerprint: {
				ID:             "mbx-2",
				KeyFingerprint: fingerprint,
				Status:         domain.MailboxStatusPendingPayment,
			},
		},
	}
	handler := NewHandler(Config{
		ChallengeAuth:    edproof.NewAuthenticator(testHMACSecret),
		KeyProofVerifier: edproof.NewVerifier(nil),
		PaymentGateway:   &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			repo,
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
		Now:    func() time.Time { return now },
	})

	body := fmt.Sprintf(`{"protocol":"imap","edproof":%q,"challenge":%q,"signature":%q}`, pubkey, challenge, sigB64)
	req := httptest.NewRequest("POST", "/v1/access/resolve", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 409 {
		t.Fatalf("expected status 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleResolveAccessRejectsUnsupportedProtocol(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)
	now := time.Now().UTC()

	challenge, _ := edproof.GenerateChallenge(pubkey, testHMACSecret, now)
	sig := ed25519.Sign(priv, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	handler := NewHandler(Config{
		ChallengeAuth:    edproof.NewAuthenticator(testHMACSecret),
		KeyProofVerifier: edproof.NewVerifier(nil),
		PaymentGateway:   &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			&httpMailboxRepo{},
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
		Now:    func() time.Time { return now },
	})

	body := fmt.Sprintf(`{"protocol":"pop3","edproof":%q,"challenge":%q,"signature":%q}`, pubkey, challenge, sigB64)
	req := httptest.NewRequest("POST", "/v1/access/resolve", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlePolarSuccessActivatesMailboxAfterVerifiedCheckout(t *testing.T) {
	repo := &httpMailboxRepo{
		byPaymentSession: map[string]*domain.Mailbox{
			"polar_1": {
				ID:               "mbx-1",
				KeyFingerprint:   "edproof:key-1",
				PaymentSessionID: "polar_1",
				Status:           domain.MailboxStatusPendingPayment,
				IMAPUsername:     "mbx_abc",
				IMAPPassword:     "secret",
			},
		},
	}
	handler := NewHandler(Config{
		PaymentGateway: httpPaymentGateway{
			session: &ports.PaymentSession{SessionID: "polar_1", Status: ports.PaymentSessionStatusSucceeded},
		},
		MailboxService: service.NewMailboxService(
			repo,
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("GET", "/v1/payments/polar/success?checkout_id=polar_1", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected ok status, got %#v", resp["status"])
	}
	if resp["mailbox_id"] != "mbx-1" {
		t.Fatalf("expected mailbox_id, got %#v", resp["mailbox_id"])
	}
	if _, ok := resp["access_token"]; ok {
		t.Fatalf("expected no access_token in response")
	}
	if _, ok := resp["payment_url"]; ok {
		t.Fatalf("expected no payment_url in response")
	}
	if repo.byPaymentSession["polar_1"].Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox activation")
	}
}

func TestHandlePolarSuccessRejectsUnpaidCheckout(t *testing.T) {
	handler := NewHandler(Config{
		PaymentGateway: httpPaymentGateway{
			session: &ports.PaymentSession{SessionID: "polar_2", Status: ports.PaymentSessionStatusOpen},
		},
		MailboxService: service.NewMailboxService(
			&httpMailboxRepo{},
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("GET", "/v1/payments/polar/success?checkout_id=polar_2", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 409 {
		t.Fatalf("expected status 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

type httpMailboxRepo struct {
	byID                          map[string]*domain.Mailbox
	byPaymentSession              map[string]*domain.Mailbox
	byKeyFingerprint              map[string]*domain.Mailbox
	byActivationTokenHash         map[string]*domain.Mailbox
	activeOrPendingByBillingEmail map[string]*domain.Mailbox
	getByIDCount                  int
	updateCount                   int
}

func (r *httpMailboxRepo) Create(_ context.Context, mailbox *domain.Mailbox) error {
	if r.byID == nil {
		r.byID = map[string]*domain.Mailbox{}
	}
	if r.byKeyFingerprint == nil {
		r.byKeyFingerprint = map[string]*domain.Mailbox{}
	}
	r.byID[mailbox.ID] = mailbox
	if mailbox.KeyFingerprint != "" {
		r.byKeyFingerprint[mailbox.KeyFingerprint] = mailbox
	}
	if r.byPaymentSession == nil {
		r.byPaymentSession = map[string]*domain.Mailbox{}
	}
	if mailbox.PaymentSessionID != "" {
		r.byPaymentSession[mailbox.PaymentSessionID] = mailbox
	}
	if r.byActivationTokenHash == nil {
		r.byActivationTokenHash = map[string]*domain.Mailbox{}
	}
	if mailbox.ActivationTokenHash != "" {
		r.byActivationTokenHash[mailbox.ActivationTokenHash] = mailbox
	}
	return nil
}

func (r *httpMailboxRepo) Update(_ context.Context, mailbox *domain.Mailbox) error {
	r.updateCount++
	if r.byID == nil {
		r.byID = map[string]*domain.Mailbox{}
	}
	r.byID[mailbox.ID] = mailbox
	if r.byPaymentSession == nil {
		r.byPaymentSession = map[string]*domain.Mailbox{}
	}
	if mailbox.PaymentSessionID != "" {
		r.byPaymentSession[mailbox.PaymentSessionID] = mailbox
	}
	if r.byActivationTokenHash == nil {
		r.byActivationTokenHash = map[string]*domain.Mailbox{}
	}
	if mailbox.ActivationTokenHash != "" {
		r.byActivationTokenHash[mailbox.ActivationTokenHash] = mailbox
	}
	if r.byKeyFingerprint == nil {
		r.byKeyFingerprint = map[string]*domain.Mailbox{}
	}
	if mailbox.KeyFingerprint != "" {
		r.byKeyFingerprint[mailbox.KeyFingerprint] = mailbox
	}
	return nil
}

func (r *httpMailboxRepo) GetByID(_ context.Context, id string) (*domain.Mailbox, error) {
	r.getByIDCount++
	if item, ok := r.byID[id]; ok {
		return item, nil
	}
	return nil, ports.ErrMailboxNotFound
}

func (r *httpMailboxRepo) ListByAccountID(_ context.Context, _ string) ([]domain.Mailbox, error) {
	return nil, nil
}

func (r *httpMailboxRepo) GetPendingByAccountID(_ context.Context, _ string) (*domain.Mailbox, error) {
	return nil, ports.ErrMailboxNotFound
}

func (r *httpMailboxRepo) ListPendingPayment(_ context.Context) ([]domain.Mailbox, error) {
	return nil, nil
}

func (r *httpMailboxRepo) GetByPaymentSessionID(_ context.Context, sessionID string) (*domain.Mailbox, error) {
	if item, ok := r.byPaymentSession[sessionID]; ok {
		return item, nil
	}
	return nil, ports.ErrMailboxNotFound
}

func (r *httpMailboxRepo) GetByActivationTokenHash(_ context.Context, tokenHash string) (*domain.Mailbox, error) {
	if item, ok := r.byActivationTokenHash[tokenHash]; ok {
		return item, nil
	}
	return nil, ports.ErrMailboxNotFound
}

func (r *httpMailboxRepo) GetByAccessToken(_ context.Context, _ string) (*domain.Mailbox, error) {
	return nil, ports.ErrMailboxNotFound
}

func (r *httpMailboxRepo) GetByKeyFingerprint(_ context.Context, keyFingerprint string) (*domain.Mailbox, error) {
	if item, ok := r.byKeyFingerprint[keyFingerprint]; ok {
		return item, nil
	}
	return nil, ports.ErrMailboxNotFound
}

func (r *httpMailboxRepo) ListActiveExpired(_ context.Context, _ time.Time) ([]domain.Mailbox, error) {
	return nil, nil
}

func (r *httpMailboxRepo) ClearActiveExpiries(_ context.Context) (int, error) {
	count := 0
	for _, mb := range r.byID {
		if mb.Status == domain.MailboxStatusActive {
			mb.ExpiresAt = nil
			count++
		}
	}
	return count, nil
}

type httpAccountRepo struct{}

func (httpAccountRepo) Create(_ context.Context, _ *domain.Account) error { return nil }
func (httpAccountRepo) GetByID(_ context.Context, _ string) (*domain.Account, error) {
	return nil, ports.ErrAccountNotFound
}
func (httpAccountRepo) GetByOwnerEmail(_ context.Context, _ string) (*domain.Account, error) {
	return nil, ports.ErrAccountNotFound
}
func (httpAccountRepo) GetByAPIToken(_ context.Context, _ string) (*domain.Account, error) {
	return nil, ports.ErrAccountNotFound
}
func (httpAccountRepo) UpdateAPIToken(_ context.Context, _ string, _ string) error { return nil }
func (httpAccountRepo) UpdateSubscriptionExpiresAt(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (httpAccountRepo) ClearSubscriptionExpiresAt(_ context.Context) (int, error) {
	return 0, nil
}

type httpPaymentGateway struct {
	session *ports.PaymentSession
}

func (httpPaymentGateway) CreatePaymentLink(_ context.Context, _ ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	return &ports.PaymentLink{SessionID: "pay-1", URL: "http://pay/1"}, nil
}

func (g httpPaymentGateway) GetPaymentSession(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
	if g.session != nil {
		return g.session, nil
	}
	return &ports.PaymentSession{SessionID: sessionID, Status: ports.PaymentSessionStatusSucceeded}, nil
}

type httpNotifier struct{}

func (httpNotifier) SendPaymentLink(_ context.Context, _ string, _ string, _ string) error {
	return nil
}
func (httpNotifier) SendActivationLink(_ context.Context, _ string, _ string, _ string) error {
	return nil
}
func (httpNotifier) SendRecoveryLink(_ context.Context, _ string, _ string) error { return nil }
func (httpNotifier) SendSupportMessage(_ context.Context, _ ports.SupportMessageParams) error {
	return nil
}

type httpTokenGenerator struct{ token string }

func (g httpTokenGenerator) NewToken(_ int) (string, error) { return g.token, nil }

type httpProvisioner struct{}

func (httpProvisioner) EnsureMailbox(_ context.Context, _ *domain.Mailbox) error { return nil }

type httpMailReader struct{}

func (httpMailReader) ListMessages(_ context.Context, _ string, _ int, _ string, _ string, _ int, _ bool, _ bool) ([]ports.IMAPMessage, error) {
	return nil, nil
}

func (httpMailReader) GetMessageByUID(_ context.Context, _ string, _ int, _ string, _ string, _ uint32, _ bool) (*ports.IMAPMessage, error) {
	return nil, nil
}

func TestHandleResolveAccessIncludesAccessToken(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)
	now := time.Now().UTC()
	future := now.Add(time.Hour)

	fingerprint, _ := edproof.FingerprintFromPubkey(pubkey)

	challenge, _ := edproof.GenerateChallenge(pubkey, testHMACSecret, now)
	sig := ed25519.Sign(priv, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	repo := &httpMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			fingerprint: {
				ID:             "mbx-1",
				KeyFingerprint: fingerprint,
				AccessToken:    "my-access-token",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(now.Add(-time.Hour)),
				ExpiresAt:      &future,
				IMAPHost:       "imap.example.com",
				IMAPPort:       143,
				IMAPUsername:   "mbx_abc",
				IMAPPassword:   "secret",
			},
		},
	}
	handler := NewHandler(Config{
		ChallengeAuth:    edproof.NewAuthenticator(testHMACSecret),
		KeyProofVerifier: edproof.NewVerifier(nil),
		PaymentGateway:   &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			repo,
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
		Now:    func() time.Time { return now },
	})

	body := fmt.Sprintf(`{"protocol":"imap","edproof":%q,"challenge":%q,"signature":%q}`, pubkey, challenge, sigB64)
	req := httptest.NewRequest("POST", "/v1/access/resolve", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["access_token"] != "my-access-token" {
		t.Fatalf("expected access_token my-access-token in resolve response, got %#v", resp["access_token"])
	}
}

func TestHandleListIMAPMessagesWorksWithoutAPIToken(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &httpMailboxRepoWithAccessToken{
		byAccessToken: map[string]*domain.Mailbox{
			"token-kb": {
				ID:           "mbx-kb",
				AccountID:    "",
				Status:       domain.MailboxStatusActive,
				PaidAt:       ptrTime(future.Add(-time.Hour)),
				ExpiresAt:    &future,
				AccessToken:  "token-kb",
				IMAPHost:     "imap",
				IMAPPort:     143,
				IMAPUsername: "u",
				IMAPPassword: "p",
			},
		},
	}
	handler := NewHandler(Config{
		PaymentGateway: &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			repo,
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("POST", "/v1/imap/messages", strings.NewReader(`{"access_token":"token-kb"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200 without X-API-Token, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

// httpMailboxRepoWithAccessToken extends httpMailboxRepo to support access-token lookup.
type httpMailboxRepoWithAccessToken struct {
	httpMailboxRepo
	byAccessToken map[string]*domain.Mailbox
}

func (r *httpMailboxRepoWithAccessToken) GetByAccessToken(_ context.Context, token string) (*domain.Mailbox, error) {
	if item, ok := r.byAccessToken[token]; ok {
		return item, nil
	}
	return nil, ports.ErrMailboxNotFound
}

// --- Challenge-response tests ---

var testHMACSecret = []byte("test-hmac-secret-must-be-at-least-32-bytes!!")

// makeSSHPubkey creates an SSH public key line from a raw ed25519 public key.
func makeSSHPubkey(pub ed25519.PublicKey) string {
	keyType := "ssh-ed25519"
	blob := make([]byte, 0, 4+len(keyType)+4+len(pub))
	typeLenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(typeLenBuf, uint32(len(keyType)))
	blob = append(blob, typeLenBuf...)
	blob = append(blob, keyType...)
	keyLenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(keyLenBuf, uint32(len(pub)))
	blob = append(blob, keyLenBuf...)
	blob = append(blob, pub...)
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " test@test"
}

func TestHandleAuthChallengeReturnsChallenge(t *testing.T) {
	t.Parallel()

	pub, _, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)

	handler := NewHandler(Config{
		ChallengeAuth: edproof.NewAuthenticator(testHMACSecret),
		Logger:        log.New(io.Discard, "", 0),
	})

	body := fmt.Sprintf(`{"public_key":%q}`, pubkey)
	req := httptest.NewRequest("POST", "/v1/auth/challenge", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["challenge"] == nil || resp["challenge"] == "" {
		t.Fatal("expected challenge in response")
	}
	if resp["expires_in"] != float64(30) {
		t.Fatalf("expected expires_in=30, got %v", resp["expires_in"])
	}
}

func TestHandleAuthChallengeRejectsInvalidKey(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Config{
		ChallengeAuth: edproof.NewAuthenticator(testHMACSecret),
		Logger:        log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("POST", "/v1/auth/challenge", strings.NewReader(`{"public_key":"not-a-key"}`))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestClaimWithChallengeResponseFullFlow(t *testing.T) {
	t.Parallel()

	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)
	now := time.Now().UTC()

	fingerprint, _ := edproof.FingerprintFromPubkey(pubkey)

	handler := NewHandler(Config{
		ChallengeAuth:    edproof.NewAuthenticator(testHMACSecret),
		KeyProofVerifier: edproof.NewVerifier(nil),
		PaymentGateway:   &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			&httpMailboxRepo{},
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
		Now:    func() time.Time { return now },
	})

	// Step 1: Get challenge
	challengeBody := fmt.Sprintf(`{"public_key":%q}`, pubkey)
	challengeReq := httptest.NewRequest("POST", "/v1/auth/challenge", strings.NewReader(challengeBody))
	challengeRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(challengeRec, challengeReq)

	if challengeRec.Code != 200 {
		t.Fatalf("challenge: expected 200, got %d body=%s", challengeRec.Code, challengeRec.Body.String())
	}

	var challengeResp map[string]any
	if err := json.Unmarshal(challengeRec.Body.Bytes(), &challengeResp); err != nil {
		t.Fatalf("decode challenge response: %v", err)
	}
	challenge := challengeResp["challenge"].(string)

	// Step 2: Sign challenge
	sig := ed25519.Sign(priv, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	// Step 3: Claim mailbox
	claimBody := fmt.Sprintf(`{"billing_email":"test@example.com","edproof":%q,"challenge":%q,"signature":%q}`, pubkey, challenge, sigB64)
	claimReq := httptest.NewRequest("POST", "/v1/mailboxes/claim", strings.NewReader(claimBody))
	claimRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(claimRec, claimReq)

	if claimRec.Code != 201 {
		t.Fatalf("claim: expected 201, got %d body=%s", claimRec.Code, claimRec.Body.String())
	}

	_ = fingerprint // used for verification only
}

func TestClaimRejectsMissingChallenge(t *testing.T) {
	t.Parallel()

	pub, _, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)

	handler := NewHandler(Config{
		ChallengeAuth:    edproof.NewAuthenticator(testHMACSecret),
		KeyProofVerifier: edproof.NewVerifier(nil),
		PaymentGateway:   &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			&httpMailboxRepo{},
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
	})

	// No challenge or signature — should be rejected
	body := fmt.Sprintf(`{"billing_email":"test@example.com","edproof":%q}`, pubkey)
	req := httptest.NewRequest("POST", "/v1/mailboxes/claim", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !strings.Contains(resp["error"], "challenge-response") {
		t.Fatalf("expected challenge-response error message, got %q", resp["error"])
	}
}

func TestResolveWithChallengeResponseFullFlow(t *testing.T) {
	t.Parallel()

	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)
	now := time.Now().UTC()

	fingerprint, _ := edproof.FingerprintFromPubkey(pubkey)
	future := now.Add(time.Hour)

	repo := &httpMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			fingerprint: {
				ID:             "mbx-cr",
				KeyFingerprint: fingerprint,
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(now.Add(-time.Hour)),
				ExpiresAt:      &future,
				IMAPHost:       "imap.example.com",
				IMAPPort:       143,
				IMAPUsername:   "mbx_cr",
				IMAPPassword:   "secret",
			},
		},
	}

	handler := NewHandler(Config{
		ChallengeAuth:    edproof.NewAuthenticator(testHMACSecret),
		KeyProofVerifier: edproof.NewVerifier(nil),
		PaymentGateway:   &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			repo,
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
		Now:    func() time.Time { return now },
	})

	// Step 1: Get challenge
	challengeBody := fmt.Sprintf(`{"public_key":%q}`, pubkey)
	challengeReq := httptest.NewRequest("POST", "/v1/auth/challenge", strings.NewReader(challengeBody))
	challengeRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(challengeRec, challengeReq)

	var challengeResp map[string]any
	if err := json.Unmarshal(challengeRec.Body.Bytes(), &challengeResp); err != nil {
		t.Fatalf("decode challenge response: %v", err)
	}
	challenge := challengeResp["challenge"].(string)

	// Step 2: Sign and resolve
	sig := ed25519.Sign(priv, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	resolveBody := fmt.Sprintf(`{"protocol":"imap","edproof":%q,"challenge":%q,"signature":%q}`, pubkey, challenge, sigB64)
	resolveReq := httptest.NewRequest("POST", "/v1/access/resolve", strings.NewReader(resolveBody))
	resolveRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(resolveRec, resolveReq)

	if resolveRec.Code != 200 {
		t.Fatalf("resolve: expected 200, got %d body=%s", resolveRec.Code, resolveRec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(resolveRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	if resp["email"] != "mbx_cr@mail.test.local" {
		t.Fatalf("expected email mbx_cr@mail.test.local, got %v", resp["email"])
	}
}

func TestResolveRejectsWrongSignature(t *testing.T) {
	t.Parallel()

	pub, _, _ := ed25519.GenerateKey(nil)
	_, wrongPriv, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)
	now := time.Now().UTC()

	handler := NewHandler(Config{
		ChallengeAuth:    edproof.NewAuthenticator(testHMACSecret),
		KeyProofVerifier: edproof.NewVerifier(nil),
		PaymentGateway:   &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			&httpMailboxRepo{},
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
		Now:    func() time.Time { return now },
	})

	// Get challenge
	challengeBody := fmt.Sprintf(`{"public_key":%q}`, pubkey)
	challengeReq := httptest.NewRequest("POST", "/v1/auth/challenge", strings.NewReader(challengeBody))
	challengeRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(challengeRec, challengeReq)

	var challengeResp map[string]any
	if err := json.Unmarshal(challengeRec.Body.Bytes(), &challengeResp); err != nil {
		t.Fatalf("decode challenge response: %v", err)
	}
	challenge := challengeResp["challenge"].(string)

	// Sign with wrong key
	sig := ed25519.Sign(wrongPriv, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	resolveBody := fmt.Sprintf(`{"protocol":"imap","edproof":%q,"challenge":%q,"signature":%q}`, pubkey, challenge, sigB64)
	resolveReq := httptest.NewRequest("POST", "/v1/access/resolve", strings.NewReader(resolveBody))
	resolveRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(resolveRec, resolveReq)

	if resolveRec.Code != 401 {
		t.Fatalf("expected 401, got %d body=%s", resolveRec.Code, resolveRec.Body.String())
	}
}

func TestHandleStripeWebhookRejectsWhenSecretNotConfigured(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Config{
		MailboxService: service.NewMailboxService(
			&httpMailboxRepo{},
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
		// StripeWebhookSecret intentionally empty
	})

	req := httptest.NewRequest("POST", "/v1/webhooks/stripe", strings.NewReader(`{}`))
	req.Header.Set("Stripe-Signature", "t=1700000000,v1=fake")
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 503 {
		t.Fatalf("expected 503 when stripe secret is empty, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlePolarWebhookRejectsWhenSecretNotConfigured(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Config{
		PolarWebhookSecret: "", // intentionally empty
		PaymentGateway:     &httpPaymentGateway{},
		MailboxService: service.NewMailboxService(
			&httpMailboxRepo{},
			&httpAccountRepo{},
			&httpPaymentGateway{},
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Logger: log.New(io.Discard, "", 0),
		Now:    func() time.Time { return time.Unix(1700000000, 0).UTC() },
	})

	req := httptest.NewRequest("POST", "/v1/webhooks/polar", strings.NewReader(`{"type":"checkout.updated","data":{"id":"polar_1"}}`))
	req.Header.Set("webhook-id", "msg_1")
	req.Header.Set("webhook-timestamp", "1700000000")
	req.Header.Set("webhook-signature", "v1,anything")
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 401 && rec.Code != 500 && rec.Code != 503 {
		t.Fatalf("expected rejection when polar secret is empty, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// --- Support message tests ---

type httpSupportMessageRepo struct {
	messages []*domain.SupportMessage
}

func (r *httpSupportMessageRepo) Create(_ context.Context, msg *domain.SupportMessage) error {
	r.messages = append(r.messages, msg)
	return nil
}

func (r *httpSupportMessageRepo) CountRecentByFingerprint(_ context.Context, fingerprint string, since time.Time) (int, error) {
	count := 0
	for _, m := range r.messages {
		if m.KeyFingerprint == fingerprint && !m.CreatedAt.Before(since) {
			count++
		}
	}
	return count, nil
}

func newSupportHandler(t *testing.T, repo *httpMailboxRepo, supportRepo *httpSupportMessageRepo) (*Handler, ed25519.PublicKey, ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)

	svc := service.NewMailboxService(
		repo,
		&httpAccountRepo{},
		&httpPaymentGateway{},
		&httpNotifier{},
		httpTokenGenerator{token: "token"},
		&httpProvisioner{},
		&httpMailReader{},
		"mail.test.local",
		"imap.test.local",
		1143,
	)
	svc.SetSupportConfig(service.SupportConfig{
		SupportEmail: "support@test.local",
		SupportRepo:  supportRepo,
	})

	handler := NewHandler(Config{
		ChallengeAuth:    edproof.NewAuthenticator(testHMACSecret),
		KeyProofVerifier: edproof.NewVerifier(nil),
		PaymentGateway:   &httpPaymentGateway{},
		MailboxService:   svc,
		Logger:           log.New(io.Discard, "", 0),
		Now:              func() time.Time { return time.Now().UTC() },
	})

	return handler, pub, priv, pubkey
}

func challengeAndSign(pubkey string, priv ed25519.PrivateKey) (string, string) {
	now := time.Now().UTC()
	challenge, _ := edproof.GenerateChallenge(pubkey, testHMACSecret, now)
	sig := ed25519.Sign(priv, []byte(challenge))
	return challenge, base64.StdEncoding.EncodeToString(sig)
}

func TestHandleSendSupportMessageHappyPath(t *testing.T) {
	t.Parallel()

	supportRepo := &httpSupportMessageRepo{}
	fingerprint := "" // will be set by the repo after claim

	handler, _, priv, pubkey := newSupportHandler(t, &httpMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{},
	}, supportRepo)

	// First, claim a mailbox so the fingerprint exists
	now := time.Now().UTC()
	claimChallenge, _ := edproof.GenerateChallenge(pubkey, testHMACSecret, now)
	claimSig := ed25519.Sign(priv, []byte(claimChallenge))
	claimBody := fmt.Sprintf(`{"billing_email":"owner@test.com","edproof":%q,"challenge":%q,"signature":%q}`,
		pubkey, claimChallenge, base64.StdEncoding.EncodeToString(claimSig))
	claimReq := httptest.NewRequest("POST", "/v1/mailboxes/claim", strings.NewReader(claimBody))
	claimRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(claimRec, claimReq)
	if claimRec.Code != 201 {
		t.Fatalf("claim failed: status %d body=%s", claimRec.Code, claimRec.Body.String())
	}
	_ = fingerprint

	// Now send a support message
	challenge, sigB64 := challengeAndSign(pubkey, priv)
	body := fmt.Sprintf(`{"edproof":%q,"challenge":%q,"signature":%q,"subject":"Help needed","body":"My mailbox is not working."}`,
		pubkey, challenge, sigB64)
	req := httptest.NewRequest("POST", "/v1/support/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "sent" {
		t.Fatalf("expected status=sent, got %q", resp["status"])
	}
	if len(supportRepo.messages) != 1 {
		t.Fatalf("expected 1 persisted message, got %d", len(supportRepo.messages))
	}
}

func TestHandleSendSupportMessageMissingFields(t *testing.T) {
	t.Parallel()

	handler, _, priv, pubkey := newSupportHandler(t, &httpMailboxRepo{}, &httpSupportMessageRepo{})

	challenge, sigB64 := challengeAndSign(pubkey, priv)

	tests := []struct {
		name string
		body string
	}{
		{"missing subject", fmt.Sprintf(`{"edproof":%q,"challenge":%q,"signature":%q,"subject":"","body":"text"}`, pubkey, challenge, sigB64)},
		{"missing body", fmt.Sprintf(`{"edproof":%q,"challenge":%q,"signature":%q,"subject":"help","body":""}`, pubkey, challenge, sigB64)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/support/messages", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			handler.Routes().ServeHTTP(rec, req)
			if rec.Code != 400 {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleSendSupportMessageExpiredChallenge(t *testing.T) {
	t.Parallel()

	handler, _, priv, pubkey := newSupportHandler(t, &httpMailboxRepo{}, &httpSupportMessageRepo{})

	// Generate a challenge from 60 seconds ago (expired)
	oldTime := time.Now().UTC().Add(-60 * time.Second)
	challenge, _ := edproof.GenerateChallenge(pubkey, testHMACSecret, oldTime)
	sig := ed25519.Sign(priv, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	body := fmt.Sprintf(`{"edproof":%q,"challenge":%q,"signature":%q,"subject":"help","body":"details"}`,
		pubkey, challenge, sigB64)
	req := httptest.NewRequest("POST", "/v1/support/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleSendSupportMessageNoMailbox(t *testing.T) {
	t.Parallel()

	// Empty repo — no mailboxes exist
	handler, _, priv, pubkey := newSupportHandler(t, &httpMailboxRepo{}, &httpSupportMessageRepo{})

	challenge, sigB64 := challengeAndSign(pubkey, priv)
	body := fmt.Sprintf(`{"edproof":%q,"challenge":%q,"signature":%q,"subject":"help","body":"details"}`,
		pubkey, challenge, sigB64)
	req := httptest.NewRequest("POST", "/v1/support/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleSendSupportMessageRateLimit(t *testing.T) {
	t.Parallel()

	supportRepo := &httpSupportMessageRepo{}
	handler, _, priv, pubkey := newSupportHandler(t, &httpMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{},
	}, supportRepo)

	// Claim a mailbox
	now := time.Now().UTC()
	claimChallenge, _ := edproof.GenerateChallenge(pubkey, testHMACSecret, now)
	claimSig := ed25519.Sign(priv, []byte(claimChallenge))
	claimBody := fmt.Sprintf(`{"billing_email":"owner@test.com","edproof":%q,"challenge":%q,"signature":%q}`,
		pubkey, claimChallenge, base64.StdEncoding.EncodeToString(claimSig))
	claimReq := httptest.NewRequest("POST", "/v1/mailboxes/claim", strings.NewReader(claimBody))
	claimRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(claimRec, claimReq)

	// Send 3 support messages (rate limit = 3/hour)
	for i := 0; i < 3; i++ {
		challenge, sigB64 := challengeAndSign(pubkey, priv)
		body := fmt.Sprintf(`{"edproof":%q,"challenge":%q,"signature":%q,"subject":"msg %d","body":"details"}`,
			pubkey, challenge, sigB64, i)
		req := httptest.NewRequest("POST", "/v1/support/messages", strings.NewReader(body))
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("message %d: expected 200, got %d body=%s", i, rec.Code, rec.Body.String())
		}
	}

	// 4th message should be rate limited
	challenge, sigB64 := challengeAndSign(pubkey, priv)
	body := fmt.Sprintf(`{"edproof":%q,"challenge":%q,"signature":%q,"subject":"too many","body":"details"}`,
		pubkey, challenge, sigB64)
	req := httptest.NewRequest("POST", "/v1/support/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 429 {
		t.Fatalf("expected 429, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func activationHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newActivationHandler(repo *httpMailboxRepo, freeMode bool) *Handler {
	svc := service.NewMailboxService(repo, &httpAccountRepo{}, &httpPaymentGateway{}, &httpNotifier{}, httpTokenGenerator{token: "x"}, &httpProvisioner{}, &httpMailReader{}, "mail.test.local", "imap.test.local", 1143)
	svc.SetFreeMode(freeMode)
	svc.SetPublicBaseURL("http://test.local")
	return NewHandler(Config{
		MailboxService: svc,
		Logger:         log.New(io.Discard, "", 0),
	})
}

func TestHandleActivateMailboxSuccess(t *testing.T) {
	repo := &httpMailboxRepo{}
	future := time.Now().UTC().Add(time.Hour)
	_ = repo.Create(context.Background(), &domain.Mailbox{
		ID:                  "mbx-1",
		Status:              domain.MailboxStatusPendingPayment,
		ActivationTokenHash: activationHash("raw-token"),
		ActivationExpiresAt: &future,
		KeyFingerprint:      "fp-1",
		IMAPUsername:        "mbx-1",
		AccessToken:         "at-1",
	})

	handler := newActivationHandler(repo, true)
	req := httptest.NewRequest("GET", "/v1/mailboxes/activate?token=raw-token", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("expected Referrer-Policy: no-referrer, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "Mailbox activated") {
		t.Fatalf("expected success page, body=%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "raw-token") {
		t.Fatalf("activation page must never render the raw token")
	}
}

func TestHandleActivateMailboxAlreadyActive(t *testing.T) {
	repo := &httpMailboxRepo{}
	now := time.Now().UTC()
	_ = repo.Create(context.Background(), &domain.Mailbox{
		ID:                  "mbx-1",
		Status:              domain.MailboxStatusActive,
		PaidAt:              &now,
		ActivationTokenHash: activationHash("raw-token"),
		KeyFingerprint:      "fp-1",
		IMAPUsername:        "mbx-1",
		AccessToken:         "at-1",
	})

	handler := newActivationHandler(repo, true)
	req := httptest.NewRequest("GET", "/v1/mailboxes/activate?token=raw-token", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already active") {
		t.Fatalf("expected already-active page, body=%s", rec.Body.String())
	}
}

func TestHandleActivateMailboxInvalidToken(t *testing.T) {
	handler := newActivationHandler(&httpMailboxRepo{}, true)
	req := httptest.NewRequest("GET", "/v1/mailboxes/activate?token=unknown", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected status 404 with recovery page, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Re-claim your mailbox") {
		t.Fatalf("expected recovery-path guidance, body=%s", rec.Body.String())
	}
}

func TestHandleActivateMailboxMissingToken(t *testing.T) {
	handler := newActivationHandler(&httpMailboxRepo{}, true)
	req := httptest.NewRequest("GET", "/v1/mailboxes/activate", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected status 400 for missing token, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleFreeModeSwitchoverRequiresAdminKey(t *testing.T) {
	repo := &httpMailboxRepo{}
	service := service.NewMailboxService(repo, &httpAccountRepo{}, &httpPaymentGateway{}, &httpNotifier{}, httpTokenGenerator{token: "token"}, &httpProvisioner{}, &httpMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)
	handler := NewHandler(Config{
		MailboxService: service,
		Logger:         log.New(io.Discard, "", 0),
	})

	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/free-mode/switchover", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without configured admin key, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleFreeModeSwitchoverRunsInFreeMode(t *testing.T) {
	future := time.Now().UTC().Add(24 * time.Hour)
	repo := &httpMailboxRepo{
		byID: map[string]*domain.Mailbox{
			"mbx-1": {
				ID:             "mbx-1",
				OwnerEmail:     "owner@example.com",
				KeyFingerprint: "edproof:key-1",
				IMAPHost:       "imap.test.local",
				IMAPPort:       1143,
				IMAPUsername:   "mbx_1",
				IMAPPassword:   "pass",
				AccessToken:    "access-1",
				Status:         domain.MailboxStatusActive,
				PaidAt:         func() *time.Time { t := time.Now().UTC().Add(-time.Hour); return &t }(),
				ExpiresAt:      &future,
			},
		},
	}
	service := service.NewMailboxService(repo, &httpAccountRepo{}, &httpPaymentGateway{}, &httpNotifier{}, httpTokenGenerator{token: "token"}, &httpProvisioner{}, &httpMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)
	handler := NewHandler(Config{
		AdminAPIKey:    "secret",
		MailboxService: service,
		Logger:         log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/free-mode/switchover", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		ActiveCleared int `json:"active_cleared"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode switchover response: %v", err)
	}
	if payload.ActiveCleared != 1 {
		t.Fatalf("expected active_cleared=1, got %d", payload.ActiveCleared)
	}
	if repo.byID["mbx-1"].ExpiresAt != nil {
		t.Fatalf("expected expiry cleared after switchover, got %v", repo.byID["mbx-1"].ExpiresAt)
	}
}

func TestHandleFreeModeSwitchoverRefusesWhenFreeModeOff(t *testing.T) {
	service := service.NewMailboxService(&httpMailboxRepo{}, &httpAccountRepo{}, &httpPaymentGateway{}, &httpNotifier{}, httpTokenGenerator{token: "token"}, &httpProvisioner{}, &httpMailReader{}, "mail.test.local", "imap.test.local", 1143)
	// freeMode defaults to false.
	handler := NewHandler(Config{
		AdminAPIKey:    "secret",
		MailboxService: service,
		Logger:         log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/free-mode/switchover", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 with free mode off, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlePolarSuccessFreeModeNoPanic(t *testing.T) {
	repo := &httpMailboxRepo{
		byPaymentSession: map[string]*domain.Mailbox{
			"polar_free_1": {
				ID:               "mbx-free",
				KeyFingerprint:   "edproof:key-1",
				PaymentSessionID: "polar_free_1",
				Status:           domain.MailboxStatusPendingPayment,
			},
		},
	}
	mailboxService := service.NewMailboxService(
		repo,
		&httpAccountRepo{},
		&httpPaymentGateway{},
		&httpNotifier{},
		httpTokenGenerator{token: "token"},
		&httpProvisioner{},
		&httpMailReader{},
		"mail.test.local",
		"imap.test.local",
		1143,
	)
	mailboxService.SetFreeMode(true)
	handler := NewHandler(Config{
		PaymentGateway: httpPaymentGateway{
			session: &ports.PaymentSession{SessionID: "polar_free_1", Status: ports.PaymentSessionStatusSucceeded},
		},
		MailboxService: mailboxService,
		Logger:         log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("GET", "/v1/payments/polar/success?checkout_id=polar_free_1", nil)
	rec := httptest.NewRecorder()

	// Must not panic on the free-mode (nil, nil) no-op from MarkMailboxPaid.
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected ok status, got %#v", resp)
	}
	if repo.byPaymentSession["polar_free_1"].Status != domain.MailboxStatusPendingPayment {
		t.Fatalf("expected mailbox to stay pending in free mode, got %s", repo.byPaymentSession["polar_free_1"].Status)
	}
}
