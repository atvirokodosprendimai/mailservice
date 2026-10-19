package service

import (
	"context"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type fakeChargeSource struct{ charges []ports.PaymentCharge }

func (f fakeChargeSource) ListPaidCharges(context.Context) ([]ports.PaymentCharge, error) {
	return f.charges, nil
}

func TestReconcileOpenCollectiveActivatesAndRenewsOnce(t *testing.T) {
	chargedAt := time.Now().UTC().Add(-35 * 24 * time.Hour).Truncate(time.Second)
	mailbox := &domain.Mailbox{ID: "mbx-1", PaymentSessionID: "tva-example", Status: domain.MailboxStatusPendingPayment, CreatedAt: chargedAt.Add(-time.Minute)}
	repo := &fakeMailboxRepo{byStripeSession: map[string]*domain.Mailbox{"tva-example": mailbox}}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, nil, &fakeMailReader{}, "mail.test", "imap.test", 993)
	first := ports.PaymentCharge{Tag: "tva-example", ChargeID: "or_1:first", ChargedAt: chargedAt}
	source := fakeChargeSource{charges: []ports.PaymentCharge{first}}
	if n, err := svc.ReconcileOpenCollectivePayments(context.Background(), source); err != nil || n != 1 {
		t.Fatalf("first reconcile = %d, %v", n, err)
	}
	if mailbox.Status != domain.MailboxStatusActive || mailbox.LastPaymentChargeID != first.ChargeID || mailbox.ExpiresAt == nil {
		t.Fatalf("activation = %+v", mailbox)
	}
	firstExpiry := *mailbox.ExpiresAt
	if !firstExpiry.Equal(chargedAt.AddDate(0, 1, 0)) {
		t.Fatalf("backfilled activation expiry = %v, want one month after charge", firstExpiry)
	}
	if n, err := svc.ReconcileOpenCollectivePayments(context.Background(), source); err != nil || n != 0 {
		t.Fatalf("repeat reconcile = %d, %v", n, err)
	}
	if !mailbox.ExpiresAt.Equal(firstExpiry) {
		t.Fatal("repeat extended expiry")
	}
	second := ports.PaymentCharge{Tag: "tva-example", ChargeID: "or_1:second", ChargedAt: chargedAt.AddDate(0, 1, 0)}
	source.charges = []ports.PaymentCharge{first, second}
	if n, err := svc.ReconcileOpenCollectivePayments(context.Background(), source); err != nil || n != 1 {
		t.Fatalf("renew reconcile = %d, %v", n, err)
	}
	if !mailbox.ExpiresAt.Equal(firstExpiry.AddDate(0, 1, 0)) {
		t.Fatalf("renewed expiry = %v", mailbox.ExpiresAt)
	}
	if n, err := svc.ReconcileOpenCollectivePayments(context.Background(), source); err != nil || n != 0 {
		t.Fatalf("repeat renewal = %d, %v", n, err)
	}
}

func TestReconcileOpenCollectiveRequiresRecordedTag(t *testing.T) {
	svc := NewMailboxService(&fakeMailboxRepo{}, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, nil, &fakeMailReader{}, "mail.test", "imap.test", 993)
	n, err := svc.ReconcileOpenCollectivePayments(context.Background(), fakeChargeSource{charges: []ports.PaymentCharge{{Tag: "tva-stranger", ChargeID: "or_1:first", ChargedAt: time.Now()}}})
	if err != nil || n != 0 {
		t.Fatalf("unrecorded tag = %d, %v", n, err)
	}
}
