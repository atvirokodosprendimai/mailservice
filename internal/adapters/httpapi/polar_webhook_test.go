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
