package httpapi

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/core/service"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type paddleCheckoutGateway struct {
	session    *ports.PaymentSession
	sessionErr error
	link       *ports.PaymentLink
}

func (g paddleCheckoutGateway) CreatePaymentLink(_ context.Context, _ ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	if g.link != nil {
		return g.link, nil
	}
	return &ports.PaymentLink{SessionID: "txn_default", URL: "https://example.com/checkout"}, nil
}

func (g paddleCheckoutGateway) GetPaymentSession(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
	if g.sessionErr != nil {
		return nil, g.sessionErr
	}
	if g.session != nil {
		return g.session, nil
	}
	return &ports.PaymentSession{SessionID: sessionID, Status: ports.PaymentSessionStatusOpen}, nil
}

func newPaddleCheckoutHandler(gateway ports.PaymentGateway, mailboxes ...*domain.Mailbox) (*httpMailboxRepo, *Handler) {
	repo := &httpMailboxRepo{
		byID:             map[string]*domain.Mailbox{},
		byPaymentSession: map[string]*domain.Mailbox{},
	}
	for _, mb := range mailboxes {
		repo.byID[mb.ID] = mb
		if mb.PaymentSessionID != "" {
			repo.byPaymentSession[mb.PaymentSessionID] = mb
		}
	}
	handler := NewHandler(Config{
		PaymentGateway:    gateway,
		PaddleClientToken: "test_client_token_123",
		PaddleEnvironment: "sandbox",
		MailboxService: service.NewMailboxService(
			repo,
			&httpAccountRepo{},
			gateway,
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
	return repo, handler
}

func TestPaddleCheckoutRendersFallbackContentForOpenSession(t *testing.T) {
	gateway := paddleCheckoutGateway{session: &ports.PaymentSession{SessionID: "txn_open1", Status: ports.PaymentSessionStatusOpen}}
	_, handler := newPaddleCheckoutHandler(gateway, &domain.Mailbox{
		ID:               "mbx-1",
		OwnerEmail:       "owner@example.com",
		IMAPUsername:     "mbx_abc",
		PaymentSessionID: "txn_open1",
		Status:           domain.MailboxStatusPendingPayment,
	})

	req := httptest.NewRequest("GET", "/v1/payments/paddle/checkout?_ptxn=txn_open1", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"mbx-1", "mbx_abc", paddleJSScriptSrc, "test_client_token_123",
		`Paddle.Environment.set("sandbox")`, "Open payment window",
		"<main>", "<h1>", `name="viewport"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q, got:\n%s", want, body)
		}
	}
}

func TestPaddleCheckoutRedirectsToSuccessForSucceededSession(t *testing.T) {
	gateway := paddleCheckoutGateway{session: &ports.PaymentSession{SessionID: "txn_done1", Status: ports.PaymentSessionStatusSucceeded}}
	_, handler := newPaddleCheckoutHandler(gateway)

	req := httptest.NewRequest("GET", "/v1/payments/paddle/checkout?_ptxn=txn_done1", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/v1/payments/paddle/success") || !strings.Contains(loc, "txn_id=txn_done1") {
		t.Fatalf("unexpected redirect location: %q", loc)
	}
}

func TestPaddleCheckoutRendersInvalidForCanceledSession(t *testing.T) {
	gateway := paddleCheckoutGateway{session: &ports.PaymentSession{SessionID: "txn_canceled1", Status: ports.PaymentSessionStatusFailed}}
	_, handler := newPaddleCheckoutHandler(gateway)

	req := httptest.NewRequest("GET", "/v1/payments/paddle/checkout?_ptxn=txn_canceled1", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no longer valid") {
		t.Fatalf("expected invalid-link copy, got:\n%s", rec.Body.String())
	}
}

func TestPaddleCheckoutRendersInvalidForNotFoundSession(t *testing.T) {
	gateway := paddleCheckoutGateway{sessionErr: ports.ErrPaymentSessionNotFound}
	_, handler := newPaddleCheckoutHandler(gateway)

	req := httptest.NewRequest("GET", "/v1/payments/paddle/checkout?_ptxn=txn_missing1", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no longer valid") {
		t.Fatalf("expected invalid-link copy, got:\n%s", rec.Body.String())
	}
}

func TestPaddleCheckoutBodyExcludesSecrets(t *testing.T) {
	gateway := paddleCheckoutGateway{session: &ports.PaymentSession{SessionID: "txn_secret1", Status: ports.PaymentSessionStatusOpen}}
	repo := &httpMailboxRepo{byID: map[string]*domain.Mailbox{}, byPaymentSession: map[string]*domain.Mailbox{}}
	mb := &domain.Mailbox{ID: "mbx-2", OwnerEmail: "o@example.com", PaymentSessionID: "txn_secret1", Status: domain.MailboxStatusPendingPayment}
	repo.byID[mb.ID] = mb
	repo.byPaymentSession[mb.PaymentSessionID] = mb

	handler := NewHandler(Config{
		PaymentGateway:      gateway,
		PaddleClientToken:   "test_visible_token",
		PaddleWebhookSecret: "pdl_ntfset_secretvalue",
		PaddleEnvironment:   "sandbox",
		MailboxService: service.NewMailboxService(
			repo, &httpAccountRepo{}, gateway, &httpNotifier{}, httpTokenGenerator{token: "token"},
			&httpProvisioner{}, &httpMailReader{}, "mail.test.local", "imap.test.local", 1143,
		),
		Logger: log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("GET", "/v1/payments/paddle/checkout?_ptxn=txn_secret1", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "test_visible_token") {
		t.Fatalf("expected client token in body, got:\n%s", body)
	}
	if strings.Contains(body, "pdl_ntfset_secretvalue") {
		t.Fatalf("webhook secret leaked into checkout body: %s", body)
	}
}

func TestPaddleCheckoutAndSuccessSetCSPHeader(t *testing.T) {
	gateway := paddleCheckoutGateway{session: &ports.PaymentSession{SessionID: "txn_csp1", Status: ports.PaymentSessionStatusOpen}}
	_, handler := newPaddleCheckoutHandler(gateway, &domain.Mailbox{
		ID: "mbx-3", PaymentSessionID: "txn_csp1", Status: domain.MailboxStatusPendingPayment,
	})

	checkoutReq := httptest.NewRequest("GET", "/v1/payments/paddle/checkout?_ptxn=txn_csp1", nil)
	checkoutRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(checkoutRec, checkoutReq)
	if csp := checkoutRec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self' https://cdn.paddle.com") {
		t.Fatalf("checkout page missing expected CSP, got %q", csp)
	}

	successReq := httptest.NewRequest("GET", "/v1/payments/paddle/success?txn_id=txn_csp1", nil)
	successRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(successRec, successReq)
	if csp := successRec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self' https://cdn.paddle.com") {
		t.Fatalf("success page missing expected CSP, got %q", csp)
	}
}

func TestPaddleCheckoutSuccessRejectsMalformedTxnID(t *testing.T) {
	_, handler := newPaddleCheckoutHandler(paddleCheckoutGateway{})

	malformed := `<script>alert(1)</script>`
	req := httptest.NewRequest("GET", "/v1/payments/paddle/success?txn_id="+url.QueryEscape(malformed), nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "script") || strings.Contains(rec.Body.String(), "alert") {
		t.Fatalf("expected no reflection of malformed input, got: %s", rec.Body.String())
	}
}

func TestPaddleCheckoutSuccessRendersProvisionalCopy(t *testing.T) {
	_, handler := newPaddleCheckoutHandler(paddleCheckoutGateway{})

	req := httptest.NewRequest("GET", "/v1/payments/paddle/success?txn_id=txn_ok123", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "activating your mailbox") {
		t.Fatalf("expected provisional copy, got: %s", body)
	}
	if strings.Contains(body, "is active") || strings.Contains(body, ">Active<") || strings.Contains(body, "ready") {
		t.Fatalf("success page must not claim an active/ready mailbox, got: %s", body)
	}
	for _, want := range []string{"<h1>", "<main>", `name="viewport"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body, got:\n%s", want, body)
		}
	}
}

// failOnWriteMailboxRepo embeds httpMailboxRepo for its read methods and
// overrides Create/Update to fail the test — a guard against the success
// route ever regaining a mutating call, since the webhook (U4) must remain
// the sole activation authority.
type failOnWriteMailboxRepo struct {
	httpMailboxRepo
	t *testing.T
}

func (r *failOnWriteMailboxRepo) Create(_ context.Context, _ *domain.Mailbox) error {
	r.t.Fatalf("unexpected mutating Create call from success route")
	return nil
}

func (r *failOnWriteMailboxRepo) Update(_ context.Context, _ *domain.Mailbox) error {
	r.t.Fatalf("unexpected mutating Update call from success route")
	return nil
}

func TestPaddleCheckoutSuccessNeverMutatesMailbox(t *testing.T) {
	repo := &failOnWriteMailboxRepo{t: t}
	gateway := paddleCheckoutGateway{}
	handler := NewHandler(Config{
		PaymentGateway: gateway,
		MailboxService: service.NewMailboxService(
			repo, &httpAccountRepo{}, gateway, &httpNotifier{}, httpTokenGenerator{token: "token"},
			&httpProvisioner{}, &httpMailReader{}, "mail.test.local", "imap.test.local", 1143,
		),
		Logger: log.New(io.Discard, "", 0),
	})

	req := httptest.NewRequest("GET", "/v1/payments/paddle/success?txn_id=txn_nomutate1", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

type recordingNotifier struct {
	paymentURL string
}

func (n *recordingNotifier) SendPaymentLink(_ context.Context, _ string, paymentURL string, _ string) error {
	n.paymentURL = paymentURL
	return nil
}
func (*recordingNotifier) SendRecoveryLink(_ context.Context, _ string, _ string) error { return nil }
func (*recordingNotifier) SendSupportMessage(_ context.Context, _ ports.SupportMessageParams) error {
	return nil
}

// TestPaddleCheckoutEmailLinksToOwnCheckoutPageWithPtxn asserts the
// payment-link email carries the actual transaction's checkout page URL
// (which PaddleGateway builds from the live transaction ID), not a URL
// constructed from static config.
func TestPaddleCheckoutEmailLinksToOwnCheckoutPageWithPtxn(t *testing.T) {
	gateway := paddleCheckoutGateway{
		link: &ports.PaymentLink{SessionID: "txn_email1", URL: "https://truevipaccess.com/v1/payments/paddle/checkout?_ptxn=txn_email1"},
	}
	notifier := &recordingNotifier{}
	mailboxService := service.NewMailboxService(
		&httpMailboxRepo{byID: map[string]*domain.Mailbox{}, byPaymentSession: map[string]*domain.Mailbox{}, byKeyFingerprint: map[string]*domain.Mailbox{}},
		&httpAccountRepo{},
		gateway,
		notifier,
		httpTokenGenerator{token: "token"},
		&httpProvisioner{},
		&httpMailReader{},
		"mail.test.local",
		"imap.test.local",
		1143,
	)

	if _, _, err := mailboxService.ClaimMailbox(context.Background(), "owner@example.com", ports.VerifiedKey{Fingerprint: "fp-email-1", Algorithm: "ed25519"}, ""); err != nil {
		t.Fatalf("ClaimMailbox failed: %v", err)
	}

	if !strings.Contains(notifier.paymentURL, "/v1/payments/paddle/checkout") || !strings.Contains(notifier.paymentURL, "_ptxn=txn_email1") {
		t.Fatalf("expected payment-link email to point at checkout page with _ptxn, got %q", notifier.paymentURL)
	}
}
