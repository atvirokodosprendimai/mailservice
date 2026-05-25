package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/core/service"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

func TestVerifyPolarWebhookAcceptsBase64EncodedSecretForm(t *testing.T) {
	secret := "polar_whs_testsecret"
	body := []byte(`{"type":"checkout.updated","data":{"id":"polar_1"}}`)
	headers := signedPolarHeaders(secret, "msg_1", 1700000000, body)

	if err := verifyPolarWebhook(secret, headers, body, time.Unix(1700000000, 0)); err != nil {
		t.Fatalf("expected webhook verification to succeed, got %v", err)
	}
}

func TestVerifyPolarWebhookRejectsBadSignature(t *testing.T) {
	secret := "polar_whs_testsecret"
	body := []byte(`{"type":"checkout.updated","data":{"id":"polar_1"}}`)
	headers := map[string]string{
		"webhook-id":        "msg_1",
		"webhook-timestamp": "1700000000",
		"webhook-signature": "v1,ZmFrZQ==",
	}

	if err := verifyPolarWebhook(secret, headers, body, time.Unix(1700000000, 0)); err == nil {
		t.Fatalf("expected webhook verification to fail")
	}
}

func TestHandlePolarWebhookActivatesMailboxAfterVerifiedSignature(t *testing.T) {
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
	paymentGateway := &httpPaymentGateway{
		session: &ports.PaymentSession{SessionID: "polar_1", Status: ports.PaymentSessionStatusSucceeded},
	}
	handler := NewHandler(Config{
		PolarWebhookSecret: "polar_whs_testsecret",
		PaymentGateway:     paymentGateway,
		MailboxService: service.NewMailboxService(
			repo,
			&httpAccountRepo{},
			paymentGateway,
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Now: func() time.Time { return time.Unix(1700000000, 0).UTC() },
	})

	body := `{"type":"checkout.updated","data":{"id":"polar_1","status":"succeeded"}}`
	req := httptest.NewRequest("POST", "/v1/webhooks/polar", strings.NewReader(body))
	for k, v := range signedPolarHeaders("polar_whs_testsecret", "msg_1", 1700000000, []byte(body)) {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if repo.byPaymentSession["polar_1"].Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox activation")
	}
}

func TestHandlePolarWebhookSubscriptionUpdatedRenewsMailbox(t *testing.T) {
	repo, handler := newPolarWebhookRenewalHandler("mbx-1")
	expiresAt := time.Date(2026, 6, 14, 12, 30, 0, 0, time.UTC)
	body := `{"type":"subscription.updated","data":{"metadata":{"mailbox_id":"mbx-1"},"current_period_end":"` + expiresAt.Format(time.RFC3339) + `"}}`
	before := time.Now()

	rec := serveSignedPolarWebhook(handler, body)
	after := time.Now()

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	mailbox := repo.byID["mbx-1"]
	if repo.updateCount != 1 {
		t.Fatalf("expected one mailbox update, got %d", repo.updateCount)
	}
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox active, got %s", mailbox.Status)
	}
	if mailbox.PaidAt == nil || mailbox.PaidAt.Before(before) || mailbox.PaidAt.After(after) {
		t.Fatalf("expected paid_at between %s and %s, got %v", before, after, mailbox.PaidAt)
	}
	if mailbox.ExpiresAt == nil || !mailbox.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at %s, got %v", expiresAt, mailbox.ExpiresAt)
	}
}

func TestHandlePolarWebhookSubscriptionUpdatedMissingMailboxIDIgnored(t *testing.T) {
	repo, handler := newPolarWebhookRenewalHandler("mbx-1")
	body := `{"type":"subscription.updated","data":{"metadata":{},"current_period_end":"2026-06-14T12:30:00Z"}}`

	rec := serveSignedPolarWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if repo.updateCount != 0 {
		t.Fatalf("expected no mailbox update, got %d", repo.updateCount)
	}
}

func TestHandlePolarWebhookSubscriptionUpdatedMissingPeriodEndIgnored(t *testing.T) {
	repo, handler := newPolarWebhookRenewalHandler("mbx-1")
	body := `{"type":"subscription.updated","data":{"metadata":{"mailbox_id":"mbx-1"}}}`

	rec := serveSignedPolarWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if repo.updateCount != 0 {
		t.Fatalf("expected no mailbox update, got %d", repo.updateCount)
	}
}

func TestHandlePolarWebhookSubscriptionUpdatedMailboxNotFoundIgnored(t *testing.T) {
	repo, handler := newPolarWebhookRenewalHandler("")
	body := `{"type":"subscription.updated","data":{"metadata":{"mailbox_id":"missing"},"current_period_end":"2026-06-14T12:30:00Z"}}`

	rec := serveSignedPolarWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if repo.updateCount != 0 {
		t.Fatalf("expected no mailbox update, got %d", repo.updateCount)
	}
}

func TestHandlePolarWebhookSubscriptionCancellationAndRevocation(t *testing.T) {
	paidAt := time.Date(2026, 5, 14, 12, 30, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 6, 14, 12, 30, 0, 0, time.UTC)

	tests := []struct {
		name              string
		mailboxID         string
		body              string
		wantStatus        string
		wantGetByIDCount  int
		wantUpdateCount   int
		wantMailboxStatus domain.MailboxStatus
	}{
		{
			name:              "subscription.canceled acks without expiring mailbox",
			mailboxID:         "mbx-1",
			body:              `{"type":"subscription.canceled","data":{"metadata":{"mailbox_id":"mbx-1"},"current_period_end":"2026-06-14T12:30:00Z"}}`,
			wantStatus:        `"status":"ok"`,
			wantGetByIDCount:  0,
			wantMailboxStatus: domain.MailboxStatusActive,
		},
		{
			name:              "subscription.uncanceled acks without expiring mailbox",
			mailboxID:         "mbx-1",
			body:              `{"type":"subscription.uncanceled","data":{"metadata":{"mailbox_id":"mbx-1"},"current_period_end":"2026-06-14T12:30:00Z"}}`,
			wantStatus:        `"status":"ok"`,
			wantGetByIDCount:  0,
			wantMailboxStatus: domain.MailboxStatusActive,
		},
		{
			name:              "subscription.revoked expires mailbox",
			mailboxID:         "mbx-1",
			body:              `{"type":"subscription.revoked","data":{"metadata":{"mailbox_id":"mbx-1"},"current_period_end":"2026-06-14T12:30:00Z"}}`,
			wantStatus:        `"status":"ok"`,
			wantGetByIDCount:  1,
			wantUpdateCount:   1,
			wantMailboxStatus: domain.MailboxStatusExpired,
		},
		{
			name:              "subscription.revoked missing mailbox id ignored",
			mailboxID:         "mbx-1",
			body:              `{"type":"subscription.revoked","data":{"metadata":{},"current_period_end":"2026-06-14T12:30:00Z"}}`,
			wantStatus:        `"status":"ignored"`,
			wantGetByIDCount:  0,
			wantMailboxStatus: domain.MailboxStatusActive,
		},
		{
			name:             "subscription.revoked mailbox not found ignored",
			body:             `{"type":"subscription.revoked","data":{"metadata":{"mailbox_id":"missing"},"current_period_end":"2026-06-14T12:30:00Z"}}`,
			wantStatus:       `"status":"ignored"`,
			mailboxID:        "",
			wantGetByIDCount: 1,
			wantUpdateCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, handler := newPolarWebhookCancellationHandler(tt.mailboxID, paidAt, expiresAt)

			rec := serveSignedPolarWebhook(handler, tt.body)

			if rec.Code != 202 {
				t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantStatus) {
				t.Fatalf("expected body to contain %s, got %s", tt.wantStatus, rec.Body.String())
			}
			if repo.getByIDCount != tt.wantGetByIDCount {
				t.Fatalf("expected %d mailbox lookups, got %d", tt.wantGetByIDCount, repo.getByIDCount)
			}
			if repo.updateCount != tt.wantUpdateCount {
				t.Fatalf("expected %d mailbox updates, got %d", tt.wantUpdateCount, repo.updateCount)
			}
			if tt.mailboxID == "" {
				return
			}
			mailbox := repo.byID[tt.mailboxID]
			if mailbox.Status != tt.wantMailboxStatus {
				t.Fatalf("expected mailbox status %s, got %s", tt.wantMailboxStatus, mailbox.Status)
			}
			if mailbox.PaidAt == nil || !mailbox.PaidAt.Equal(paidAt) {
				t.Fatalf("expected paid_at to remain %s, got %v", paidAt, mailbox.PaidAt)
			}
			if mailbox.ExpiresAt == nil || !mailbox.ExpiresAt.Equal(expiresAt) {
				t.Fatalf("expected expires_at to remain %s, got %v", expiresAt, mailbox.ExpiresAt)
			}
		})
	}
}

func TestHandlePolarWebhookOrderCreatedRenewsMailbox(t *testing.T) {
	repo, handler := newPolarWebhookRenewalHandler("mbx-1")
	body := `{"type":"order.created","data":{"metadata":{"mailbox_id":"mbx-1"},"current_period_end":"2026-06-14T12:30:00Z"}}`

	rec := serveSignedPolarWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if repo.updateCount != 1 {
		t.Fatalf("expected one mailbox update, got %d", repo.updateCount)
	}
}

func TestHandlePolarWebhookOrderPaidRenewsMailbox(t *testing.T) {
	repo, handler := newPolarWebhookRenewalHandler("mbx-1")
	body := `{"type":"order.paid","data":{"metadata":{"mailbox_id":"mbx-1"},"current_period_end":"2026-06-14T12:30:00Z"}}`

	rec := serveSignedPolarWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if repo.updateCount != 1 {
		t.Fatalf("expected one mailbox update, got %d", repo.updateCount)
	}
}

func TestHandlePolarWebhookRejectsInvalidSignature(t *testing.T) {
	handler := NewHandler(Config{
		PolarWebhookSecret: "polar_whs_testsecret",
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
		Now: func() time.Time { return time.Unix(1700000000, 0).UTC() },
	})

	req := httptest.NewRequest("POST", "/v1/webhooks/polar", strings.NewReader(`{"type":"checkout.updated","data":{"id":"polar_1"}}`))
	req.Header.Set("webhook-id", "msg_1")
	req.Header.Set("webhook-timestamp", "1700000000")
	req.Header.Set("webhook-signature", "v1,ZmFrZQ==")
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected status 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func newPolarWebhookRenewalHandler(mailboxID string) (*httpMailboxRepo, *Handler) {
	repo := &httpMailboxRepo{byID: map[string]*domain.Mailbox{}}
	if mailboxID != "" {
		repo.byID[mailboxID] = &domain.Mailbox{
			ID:             mailboxID,
			KeyFingerprint: "edproof:key-1",
			Status:         domain.MailboxStatusExpired,
			IMAPUsername:   "mbx_abc",
			IMAPPassword:   "secret",
		}
	}
	paymentGateway := &httpPaymentGateway{}
	handler := NewHandler(Config{
		PolarWebhookSecret: "polar_whs_testsecret",
		PaymentGateway:     paymentGateway,
		MailboxService: service.NewMailboxService(
			repo,
			&httpAccountRepo{},
			paymentGateway,
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Now: func() time.Time { return time.Unix(1700000000, 0).UTC() },
	})
	return repo, handler
}

func newPolarWebhookCancellationHandler(mailboxID string, paidAt time.Time, expiresAt time.Time) (*httpMailboxRepo, *Handler) {
	repo := &httpMailboxRepo{byID: map[string]*domain.Mailbox{}}
	if mailboxID != "" {
		repo.byID[mailboxID] = &domain.Mailbox{
			ID:             mailboxID,
			KeyFingerprint: "edproof:key-1",
			Status:         domain.MailboxStatusActive,
			PaidAt:         &paidAt,
			ExpiresAt:      &expiresAt,
			IMAPUsername:   "mbx_abc",
			IMAPPassword:   "secret",
		}
	}
	paymentGateway := &httpPaymentGateway{}
	handler := NewHandler(Config{
		PolarWebhookSecret: "polar_whs_testsecret",
		PaymentGateway:     paymentGateway,
		MailboxService: service.NewMailboxService(
			repo,
			&httpAccountRepo{},
			paymentGateway,
			&httpNotifier{},
			httpTokenGenerator{token: "token"},
			&httpProvisioner{},
			&httpMailReader{},
			"mail.test.local",
			"imap.test.local",
			1143,
		),
		Now: func() time.Time { return time.Unix(1700000000, 0).UTC() },
	})
	return repo, handler
}

func serveSignedPolarWebhook(handler *Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/v1/webhooks/polar", strings.NewReader(body))
	for k, v := range signedPolarHeaders("polar_whs_testsecret", "msg_1", 1700000000, []byte(body)) {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}

func signedPolarHeaders(secret string, msgID string, timestamp int64, body []byte) map[string]string {
	signingSecret := base64.StdEncoding.EncodeToString([]byte(secret))
	timestampString := strconv.FormatInt(timestamp, 10)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = mac.Write([]byte(msgID + "." + timestampString + "." + string(body)))
	return map[string]string{
		"webhook-id":        msgID,
		"webhook-timestamp": timestampString,
		"webhook-signature": "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil)),
	}
}
