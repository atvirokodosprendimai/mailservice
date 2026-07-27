package httpapi

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/service"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

// fixedPaddleNow is the business-clock time injected via Config.Now for all
// scenarios below. It is independent of the wall-clock time used to sign
// the webhook headers (verifyPaddleWebhook checks signature freshness
// against real time via the Paddle SDK verifier, not the injected clock).
var fixedPaddleNow = time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

func newPaddleWebhookHandler(mailboxes ...*domain.Mailbox) (*httpMailboxRepo, *Handler) {
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
	handler := NewHandler(Config{
		PaddleWebhookSecret: "pdl_ntfset_testsecret",
		PaymentGateway:      paymentGateway,
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
	return repo, handler
}

func servePaddleWebhook(handler *Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/v1/webhooks/paddle", strings.NewReader(body))
	req.Header.Set("Paddle-Signature", signedPaddleHeader("pdl_ntfset_testsecret", time.Now().Unix(), []byte(body)))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}

func TestHandlePaddleWebhookSubscriptionCreatedActivatesNotYetPaidMailbox(t *testing.T) {
	repo, handler := newPaddleWebhookHandler(&domain.Mailbox{
		ID:               "mbx-1",
		KeyFingerprint:   "edproof:key-1",
		PaymentSessionID: "txn_original",
		Status:           domain.MailboxStatusPendingPayment,
		IMAPUsername:     "mbx_abc",
		IMAPPassword:     "secret",
	})

	body := `{
		"event_id":"evt_1",
		"event_type":"subscription.created",
		"occurred_at":"2026-07-27T10:00:00Z",
		"data":{
			"id":"sub_1",
			"transaction_id":"txn_original",
			"status":"active",
			"custom_data":{"mailbox_id":"mbx-1"}
		}
	}`

	rec := servePaddleWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	mailbox := repo.byID["mbx-1"]
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox activated, got status=%s", mailbox.Status)
	}
	if mailbox.SubscriptionID != "sub_1" {
		t.Fatalf("expected subscription_id recorded as sub_1, got %q", mailbox.SubscriptionID)
	}
	if mailbox.LastPaymentEventID != "evt_1" {
		t.Fatalf("expected last_payment_event_id evt_1, got %q", mailbox.LastPaymentEventID)
	}
}

func TestHandlePaddleWebhookTransactionCompletedOnNotYetPaidMailboxActivatesNotRenews(t *testing.T) {
	repo, handler := newPaddleWebhookHandler(&domain.Mailbox{
		ID:               "mbx-1",
		KeyFingerprint:   "edproof:key-1",
		PaymentSessionID: "txn_original",
		Status:           domain.MailboxStatusPendingPayment,
		IMAPUsername:     "mbx_abc",
		IMAPPassword:     "secret",
	})

	body := `{
		"event_id":"evt_2",
		"event_type":"transaction.completed",
		"occurred_at":"2026-07-27T10:05:00Z",
		"data":{
			"id":"txn_original",
			"status":"completed",
			"subscription_id":"sub_1",
			"custom_data":{"mailbox_id":"mbx-1"},
			"billing_period":{"starts_at":"2026-07-27T10:00:00Z","ends_at":"2026-08-27T10:00:00Z"}
		}
	}`

	rec := servePaddleWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	mailbox := repo.byID["mbx-1"]
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox activated via MarkMailboxPaid, got status=%s", mailbox.Status)
	}
	// MarkMailboxPaid stamps paid_at with the real wall clock, not the
	// injected fixedPaddleNow that RenewMailbox would have used — this is
	// the discriminator that proves MarkMailboxPaid ran, not RenewMailbox.
	if mailbox.PaidAt == nil || mailbox.PaidAt.Equal(fixedPaddleNow) {
		t.Fatalf("expected activation-path paid_at (real clock), not renewal's injected clock; got %v", mailbox.PaidAt)
	}
}

func TestHandlePaddleWebhookTransactionCompletedOnAlreadyPaidMailboxRenews(t *testing.T) {
	expiresAt := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	repo, handler := newPaddleWebhookHandler(&domain.Mailbox{
		ID:               "mbx-1",
		KeyFingerprint:   "edproof:key-1",
		PaymentSessionID: "txn_original",
		SubscriptionID:   "sub_1",
		Status:           domain.MailboxStatusActive,
		PaidAt:           ptrTime(time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)),
		ExpiresAt:        &expiresAt,
		IMAPUsername:     "mbx_abc",
		IMAPPassword:     "secret",
	})

	newExpiry := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	body := `{
		"event_id":"evt_3",
		"event_type":"transaction.completed",
		"occurred_at":"2026-07-27T10:05:00Z",
		"data":{
			"id":"txn_renewal",
			"status":"completed",
			"subscription_id":"sub_1",
			"billing_period":{"starts_at":"2026-07-27T10:00:00Z","ends_at":"` + newExpiry.Format(time.RFC3339) + `"}
		}
	}`

	rec := servePaddleWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	mailbox := repo.byID["mbx-1"]
	if mailbox.ExpiresAt == nil || !mailbox.ExpiresAt.Equal(newExpiry) {
		t.Fatalf("expected expires_at renewed to %s, got %v", newExpiry, mailbox.ExpiresAt)
	}
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox to remain active, got %s", mailbox.Status)
	}
}

func TestHandlePaddleWebhookTransactionCompletedUnresolvedMailboxIgnored(t *testing.T) {
	repo, handler := newPaddleWebhookHandler()

	body := `{
		"event_id":"evt_4",
		"event_type":"transaction.completed",
		"occurred_at":"2026-07-27T10:05:00Z",
		"data":{
			"id":"txn_unknown",
			"status":"completed",
			"subscription_id":"sub_unknown",
			"custom_data":{}
		}
	}`

	rec := servePaddleWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ignored"`) {
		t.Fatalf("expected ignored status, got %s", rec.Body.String())
	}
	if repo.updateCount != 0 {
		t.Fatalf("expected no mailbox mutation, got %d updates", repo.updateCount)
	}
}

func TestHandlePaddleWebhookSubscriptionCanceledElapsedExpiresImmediately(t *testing.T) {
	expiresAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	repo, handler := newPaddleWebhookHandler(&domain.Mailbox{
		ID:               "mbx-1",
		KeyFingerprint:   "edproof:key-1",
		PaymentSessionID: "txn_original",
		SubscriptionID:   "sub_1",
		Status:           domain.MailboxStatusActive,
		PaidAt:           ptrTime(time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)),
		ExpiresAt:        &expiresAt,
		IMAPUsername:     "mbx_abc",
		IMAPPassword:     "secret",
	})

	// current_billing_period.ends_at is in the past relative to fixedPaddleNow (2026-07-27T12:00:00Z).
	body := `{
		"event_id":"evt_5",
		"event_type":"subscription.canceled",
		"occurred_at":"2026-07-27T11:00:00Z",
		"data":{
			"id":"sub_1",
			"status":"canceled",
			"custom_data":{"mailbox_id":"mbx-1"},
			"current_billing_period":{"starts_at":"2026-06-27T09:00:00Z","ends_at":"2026-07-27T09:00:00Z"}
		}
	}`

	rec := servePaddleWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	mailbox := repo.byID["mbx-1"]
	if mailbox.Status != domain.MailboxStatusExpired {
		t.Fatalf("expected mailbox expired immediately, got status=%s", mailbox.Status)
	}
}

func TestHandlePaddleWebhookSubscriptionCanceledNotElapsedSchedulesExpiry(t *testing.T) {
	originalExpiresAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	repo, handler := newPaddleWebhookHandler(&domain.Mailbox{
		ID:               "mbx-1",
		KeyFingerprint:   "edproof:key-1",
		PaymentSessionID: "txn_original",
		SubscriptionID:   "sub_1",
		Status:           domain.MailboxStatusActive,
		PaidAt:           ptrTime(time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)),
		ExpiresAt:        &originalExpiresAt,
		IMAPUsername:     "mbx_abc",
		IMAPPassword:     "secret",
	})

	periodEnd := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	body := `{
		"event_id":"evt_6",
		"event_type":"subscription.canceled",
		"occurred_at":"2026-07-27T11:00:00Z",
		"data":{
			"id":"sub_1",
			"status":"active",
			"custom_data":{"mailbox_id":"mbx-1"},
			"current_billing_period":{"starts_at":"2026-07-27T09:00:00Z","ends_at":"` + periodEnd.Format(time.RFC3339) + `"}
		}
	}`

	rec := servePaddleWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	mailbox := repo.byID["mbx-1"]
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox not immediately revoked, got status=%s", mailbox.Status)
	}
	if mailbox.ExpiresAt == nil || !mailbox.ExpiresAt.Equal(periodEnd) {
		t.Fatalf("expected expiry scheduled for period end %s, got %v", periodEnd, mailbox.ExpiresAt)
	}
}

func TestHandlePaddleWebhookStaleOccurredAtIgnored(t *testing.T) {
	lastEventAt := time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	repo, handler := newPaddleWebhookHandler(&domain.Mailbox{
		ID:                 "mbx-1",
		KeyFingerprint:     "edproof:key-1",
		PaymentSessionID:   "txn_original",
		SubscriptionID:     "sub_1",
		Status:             domain.MailboxStatusActive,
		PaidAt:             ptrTime(time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)),
		ExpiresAt:          &expiresAt,
		LastPaymentEventID: "evt_prev",
		LastPaymentEventAt: &lastEventAt,
		IMAPUsername:       "mbx_abc",
		IMAPPassword:       "secret",
	})

	// occurred_at (10:00) is older than the mailbox's last_payment_event_at (11:00).
	body := `{
		"event_id":"evt_stale",
		"event_type":"transaction.completed",
		"occurred_at":"2026-07-27T10:00:00Z",
		"data":{
			"id":"txn_stale",
			"subscription_id":"sub_1",
			"billing_period":{"starts_at":"2026-07-27T09:00:00Z","ends_at":"2026-09-27T09:00:00Z"}
		}
	}`

	rec := servePaddleWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ignored"`) {
		t.Fatalf("expected ignored status, got %s", rec.Body.String())
	}
	mailbox := repo.byID["mbx-1"]
	if !mailbox.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at unchanged at %s, got %v", expiresAt, mailbox.ExpiresAt)
	}
	if mailbox.LastPaymentEventID != "evt_prev" {
		t.Fatalf("expected last_payment_event_id unchanged, got %q", mailbox.LastPaymentEventID)
	}
	if repo.updateCount != 0 {
		t.Fatalf("expected no mailbox mutation, got %d updates", repo.updateCount)
	}
}

func TestHandlePaddleWebhookDuplicateEventIDIsNoOp(t *testing.T) {
	lastEventAt := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	repo, handler := newPaddleWebhookHandler(&domain.Mailbox{
		ID:                 "mbx-1",
		KeyFingerprint:     "edproof:key-1",
		PaymentSessionID:   "txn_original",
		SubscriptionID:     "sub_1",
		Status:             domain.MailboxStatusActive,
		PaidAt:             ptrTime(time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)),
		ExpiresAt:          &expiresAt,
		LastPaymentEventID: "evt_dup",
		LastPaymentEventAt: &lastEventAt,
		IMAPUsername:       "mbx_abc",
		IMAPPassword:       "secret",
	})

	// Same event_id as already recorded, later occurred_at and a fresh
	// billing_period that WOULD extend expires_at if it were applied.
	body := `{
		"event_id":"evt_dup",
		"event_type":"transaction.completed",
		"occurred_at":"2026-07-27T12:00:00Z",
		"data":{
			"id":"txn_retry",
			"subscription_id":"sub_1",
			"billing_period":{"starts_at":"2026-07-27T09:00:00Z","ends_at":"2026-09-27T09:00:00Z"}
		}
	}`

	rec := servePaddleWebhook(handler, body)

	if rec.Code != 202 {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	mailbox := repo.byID["mbx-1"]
	if !mailbox.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at NOT double-extended, still %s, got %v", expiresAt, mailbox.ExpiresAt)
	}
	if repo.updateCount != 0 {
		t.Fatalf("expected duplicate delivery to be a pure no-op, got %d updates", repo.updateCount)
	}
}

func TestHandlePaddleWebhookUnrecognizedEventTypeIgnored(t *testing.T) {
	repo, handler := newPaddleWebhookHandler(&domain.Mailbox{
		ID:               "mbx-1",
		KeyFingerprint:   "edproof:key-1",
		PaymentSessionID: "txn_original",
		SubscriptionID:   "sub_1",
		Status:           domain.MailboxStatusActive,
		IMAPUsername:     "mbx_abc",
		IMAPPassword:     "secret",
	})

	body := `{
		"event_id":"evt_7",
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
	if !strings.Contains(rec.Body.String(), `"status":"ignored"`) {
		t.Fatalf("expected ignored status, got %s", rec.Body.String())
	}
	if repo.updateCount != 0 {
		t.Fatalf("expected no mailbox mutation, got %d updates", repo.updateCount)
	}
}

func TestHandlePaddleWebhookRejectsInvalidSignature(t *testing.T) {
	_, handler := newPaddleWebhookHandler()

	body := `{"event_id":"evt_1","event_type":"transaction.completed","data":{"id":"txn_1"}}`
	req := httptest.NewRequest("POST", "/v1/webhooks/paddle", strings.NewReader(body))
	req.Header.Set("Paddle-Signature", "ts="+strconv.FormatInt(time.Now().Unix(), 10)+";h1="+strings.Repeat("0", 64))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected status 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}
