package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestHandleClaimMailboxCreatesPendingMailbox(t *testing.T) {
	repo := &httpMailboxRepo{}
	handler := NewHandler(Config{
		KeyProofVerifier: fakeKeyProofVerifier{
			key: &ports.VerifiedKey{Fingerprint: "edproof:key-1", Algorithm: "ed25519"},
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
	return nil
}

func (r *httpMailboxRepo) Update(_ context.Context, mailbox *domain.Mailbox) error {
	if r.byID == nil {
		r.byID = map[string]*domain.Mailbox{}
	}
	r.byID[mailbox.ID] = mailbox
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

func (r *httpMailboxRepo) GetByStripeSessionID(_ context.Context, _ string) (*domain.Mailbox, error) {
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

type httpPaymentGateway struct{}

func (httpPaymentGateway) CreatePaymentLink(_ context.Context, _ ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	return &ports.PaymentLink{SessionID: "pay-1", URL: "http://pay/1"}, nil
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

func ptrTime(value time.Time) *time.Time {
	return &value
}
