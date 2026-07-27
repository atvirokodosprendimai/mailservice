package httpapi

import (
	"context"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/service"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
	"github.com/atvirokodosprendimai/mailservice/internal/platform/metrics"
)

func newPaddleWebhookHandlerWithMetrics(mailboxes ...*domain.Mailbox) (*httpMailboxRepo, *Handler, *metrics.Registry) {
	repo := &httpMailboxRepo{
		byID:             map[string]*domain.Mailbox{},
		byPaymentSession: map[string]*domain.Mailbox{},
		bySubscriptionID: map[string]*domain.Mailbox{},
	}
	for _, mb := range mailboxes {
		repo.byID[mb.ID] = mb
		if mb.PaymentSessionID != "" {
			repo.byPaymentSession[mb.PaymentSessionID] = mb
		}
		if mb.SubscriptionID != "" {
			repo.bySubscriptionID[mb.SubscriptionID] = mb
		}
	}

	paymentGateway := &httpPaymentGateway{}
	registry := metrics.NewRegistry(context.Background())
	handler := NewHandler(Config{
		PaddleWebhookSecret: "pdl_ntfset_testsecret",
		PaymentGateway:      paymentGateway,
		Metrics:             registry,
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
		Now: func() time.Time { return fixedPaddleNow },
	})
	return repo, handler, registry
}

func TestHandlePaddleWebhookTransactionCompletedIncrementsWebhookReceivedBuckets(t *testing.T) {
	_, handler, registry := newPaddleWebhookHandlerWithMetrics(&domain.Mailbox{
		ID:               "mbx-1",
		KeyFingerprint:   "edproof:key-1",
		PaymentSessionID: "txn_original",
		SubscriptionID:   "sub_1",
		Status:           domain.MailboxStatusActive,
		IMAPUsername:     "mbx_abc",
		IMAPPassword:     "secret",
	})

	body := `{
		"event_id":"evt_metrics_1",
		"event_type":"transaction.completed",
		"occurred_at":"2026-07-27T10:05:00Z",
		"data":{
			"id":"txn_renewal",
			"status":"completed",
			"subscription_id":"sub_1",
			"billing_period":{"starts_at":"2026-07-27T10:00:00Z","ends_at":"2026-08-27T10:00:00Z"}
		}
	}`

	rec := servePaddleWebhook(handler, body)
	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}

	if got := registry.Counter("webhook_received").Sum24h(); got != 1 {
		t.Fatalf("webhook_received = %d, want 1", got)
	}
	if got := registry.Counter("webhook_received_transaction_completed").Sum24h(); got != 1 {
		t.Fatalf("webhook_received_transaction_completed = %d, want 1", got)
	}
	if got := registry.Counter("webhook_received_subscription_created").Sum24h(); got != 0 {
		t.Fatalf("webhook_received_subscription_created = %d, want 0", got)
	}
	if got := registry.Counter("webhook_received_other").Sum24h(); got != 0 {
		t.Fatalf("webhook_received_other = %d, want 0", got)
	}
}

func TestHandlePaddleWebhookUnrecognizedEventTypeIncrementsWebhookReceivedOther(t *testing.T) {
	_, handler, registry := newPaddleWebhookHandlerWithMetrics(&domain.Mailbox{
		ID:               "mbx-1",
		KeyFingerprint:   "edproof:key-1",
		PaymentSessionID: "txn_original",
		SubscriptionID:   "sub_1",
		Status:           domain.MailboxStatusActive,
		IMAPUsername:     "mbx_abc",
		IMAPPassword:     "secret",
	})

	body := `{
		"event_id":"evt_metrics_2",
		"event_type":"subscription.paused",
		"occurred_at":"2026-07-27T10:00:00Z",
		"data":{
			"id":"sub_1",
			"custom_data":{"mailbox_id":"mbx-1"}
		}
	}`

	rec := servePaddleWebhook(handler, body)
	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}

	if got := registry.Counter("webhook_received").Sum24h(); got != 1 {
		t.Fatalf("webhook_received = %d, want 1", got)
	}
	if got := registry.Counter("webhook_received_other").Sum24h(); got != 1 {
		t.Fatalf("webhook_received_other = %d, want 1", got)
	}
}

func TestHandlePaddleWebhookInvalidSignatureIncrementsWebhookVerificationFailed(t *testing.T) {
	_, handler, registry := newPaddleWebhookHandlerWithMetrics()

	body := `{"event_id":"evt_1","event_type":"transaction.completed","occurred_at":"2026-07-27T10:00:00Z","data":{"id":"txn_1"}}`
	req := httptest.NewRequest("POST", "/v1/webhooks/paddle", strings.NewReader(body))
	req.Header.Set("Paddle-Signature", "ts="+strconv.FormatInt(time.Now().Unix(), 10)+";h1="+strings.Repeat("0", 64))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected status 401, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := registry.Counter("webhook_verification_failed").Sum24h(); got != 1 {
		t.Fatalf("webhook_verification_failed = %d, want 1", got)
	}
	if got := registry.Counter("webhook_received").Sum24h(); got != 0 {
		t.Fatalf("webhook_received = %d, want 0 (verification failed before parsing)", got)
	}
}
