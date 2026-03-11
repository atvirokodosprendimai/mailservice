package httpapi

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/adapters/identity/edproof"
	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/core/service"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
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
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("expected no-store cache control, got %q", got)
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

func TestHandleClaimMailboxCreatesPendingMailbox(t *testing.T) {
	repo := &httpMailboxRepo{}
	handler := NewHandler(Config{
		KeyProofVerifier: fakeKeyProofVerifier{
			key: &ports.VerifiedKey{Fingerprint: "edproof:key-1", Algorithm: "ed25519"},
		},
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

	req := httptest.NewRequest("POST", "/v1/mailboxes/claim", strings.NewReader(`{"billing_email":"billing@example.com","edproof":"proof"}`))
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

func TestHandleClaimMailboxRejectsInvalidProof(t *testing.T) {
	handler := NewHandler(Config{
		KeyProofVerifier: fakeKeyProofVerifier{err: ports.ErrInvalidKeyProof},
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

	if rec.Code != 401 {
		t.Fatalf("expected status 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleResolveAccessReturnsIMAPDetailsForValidKey(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &httpMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:             "mbx-1",
				KeyFingerprint: "edproof:key-1",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(future.Add(-time.Hour)),
				ExpiresAt:      &future,
				IMAPHost:       "imap.example.com",
				IMAPPort:       143,
				IMAPUsername:   "mbx_abc",
				IMAPPassword:   "secret",
			},
		},
	}
	handler := NewHandler(Config{
		KeyProofVerifier: fakeKeyProofVerifier{
			key: &ports.VerifiedKey{Fingerprint: "edproof:key-1", Algorithm: "ed25519"},
		},
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

	req := httptest.NewRequest("POST", "/v1/access/resolve", strings.NewReader(`{"protocol":"imap","edproof":"proof"}`))
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
	repo := &httpMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-2": {
				ID:             "mbx-2",
				KeyFingerprint: "edproof:key-2",
				Status:         domain.MailboxStatusPendingPayment,
			},
		},
	}
	handler := NewHandler(Config{
		KeyProofVerifier: fakeKeyProofVerifier{
			key: &ports.VerifiedKey{Fingerprint: "edproof:key-2", Algorithm: "ed25519"},
		},
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

	req := httptest.NewRequest("POST", "/v1/access/resolve", strings.NewReader(`{"protocol":"imap","edproof":"proof"}`))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 409 {
		t.Fatalf("expected status 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleResolveAccessRejectsUnsupportedProtocol(t *testing.T) {
	handler := NewHandler(Config{
		KeyProofVerifier: fakeKeyProofVerifier{
			key: &ports.VerifiedKey{Fingerprint: "edproof:key-2", Algorithm: "ed25519"},
		},
		PaymentGateway: &httpPaymentGateway{},
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

	req := httptest.NewRequest("POST", "/v1/access/resolve", strings.NewReader(`{"protocol":"pop3","edproof":"proof"}`))
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

type fakeKeyProofVerifier struct {
	key *ports.VerifiedKey
	err error
}

func (f fakeKeyProofVerifier) Verify(_ context.Context, _ string) (*ports.VerifiedKey, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.key, nil
}

type httpMailboxRepo struct {
	byID             map[string]*domain.Mailbox
	byPaymentSession map[string]*domain.Mailbox
	byKeyFingerprint map[string]*domain.Mailbox
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
	return nil
}

func (r *httpMailboxRepo) Update(_ context.Context, mailbox *domain.Mailbox) error {
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
	if r.byKeyFingerprint == nil {
		r.byKeyFingerprint = map[string]*domain.Mailbox{}
	}
	if mailbox.KeyFingerprint != "" {
		r.byKeyFingerprint[mailbox.KeyFingerprint] = mailbox
	}
	return nil
}

func (r *httpMailboxRepo) GetByID(_ context.Context, id string) (*domain.Mailbox, error) {
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

func (r *httpMailboxRepo) GetByPaymentSessionID(_ context.Context, sessionID string) (*domain.Mailbox, error) {
	if item, ok := r.byPaymentSession[sessionID]; ok {
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
func (httpNotifier) SendRecoveryLink(_ context.Context, _ string, _ string) error { return nil }

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
	future := time.Now().UTC().Add(time.Hour)
	repo := &httpMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:             "mbx-1",
				KeyFingerprint: "edproof:key-1",
				AccessToken:    "my-access-token",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(future.Add(-time.Hour)),
				ExpiresAt:      &future,
				IMAPHost:       "imap.example.com",
				IMAPPort:       143,
				IMAPUsername:   "mbx_abc",
				IMAPPassword:   "secret",
			},
		},
	}
	handler := NewHandler(Config{
		KeyProofVerifier: fakeKeyProofVerifier{
			key: &ports.VerifiedKey{Fingerprint: "edproof:key-1", Algorithm: "ed25519"},
		},
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

	req := httptest.NewRequest("POST", "/v1/access/resolve", strings.NewReader(`{"protocol":"imap","edproof":"proof"}`))
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
		EdproofHMACSecret: testHMACSecret,
		Logger:            log.New(io.Discard, "", 0),
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
		EdproofHMACSecret: testHMACSecret,
		Logger:            log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("POST", "/v1/auth/challenge", strings.NewReader(`{"public_key":"not-a-key"}`))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleAuthChallengeRejectsWhenNotConfigured(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Config{
		Logger: log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("POST", "/v1/auth/challenge", strings.NewReader(`{"public_key":"ssh-ed25519 AAAA test"}`))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 503 {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestClaimWithChallengeResponseFullFlow(t *testing.T) {
	t.Parallel()

	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)
	now := time.Now().UTC()

	fingerprint, _ := edproof.FingerprintFromPubkey(pubkey)

	handler := NewHandler(Config{
		EdproofHMACSecret: testHMACSecret,
		KeyProofVerifier:  edproof.NewVerifier(nil),
		PaymentGateway:    &httpPaymentGateway{},
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
	json.Unmarshal(challengeRec.Body.Bytes(), &challengeResp)
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

func TestClaimRejectsMissingChallengeWhenConfigured(t *testing.T) {
	t.Parallel()

	pub, _, _ := ed25519.GenerateKey(nil)
	pubkey := makeSSHPubkey(pub)

	handler := NewHandler(Config{
		EdproofHMACSecret: testHMACSecret,
		KeyProofVerifier:  edproof.NewVerifier(nil),
		PaymentGateway:    &httpPaymentGateway{},
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
	json.Unmarshal(rec.Body.Bytes(), &resp)
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
				IMAPUsername:    "mbx_cr",
				IMAPPassword:   "secret",
			},
		},
	}

	handler := NewHandler(Config{
		EdproofHMACSecret: testHMACSecret,
		KeyProofVerifier:  edproof.NewVerifier(nil),
		PaymentGateway:    &httpPaymentGateway{},
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
	json.Unmarshal(challengeRec.Body.Bytes(), &challengeResp)
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
	json.Unmarshal(resolveRec.Body.Bytes(), &resp)
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
		EdproofHMACSecret: testHMACSecret,
		KeyProofVerifier:  edproof.NewVerifier(nil),
		PaymentGateway:    &httpPaymentGateway{},
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
	json.Unmarshal(challengeRec.Body.Bytes(), &challengeResp)
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
