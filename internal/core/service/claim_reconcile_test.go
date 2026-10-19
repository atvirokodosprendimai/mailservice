package service

import (
	"context"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type claimChargeGateway struct {
	charges []ports.PaymentCharge
	calls   int
}

func (g *claimChargeGateway) CreatePaymentLink(_ context.Context, _ ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	return &ports.PaymentLink{SessionID: "tva-new", URL: "https://opencollective.com/checkout?tags=tva-new"}, nil
}

func (g *claimChargeGateway) GetPaymentSession(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
	return &ports.PaymentSession{SessionID: sessionID, Status: ports.PaymentSessionStatusOpen}, nil
}

func (g *claimChargeGateway) ListPaidCharges(_ context.Context) ([]ports.PaymentCharge, error) {
	g.calls++
	return g.charges, nil
}

type claimReconcileRepo struct {
	*fakeMailboxRepo
}

func (r *claimReconcileRepo) GetByID(_ context.Context, id string) (*domain.Mailbox, error) {
	for _, mailbox := range r.byKeyFingerprint {
		if mailbox.ID == id {
			return mailbox, nil
		}
	}
	return nil, ports.ErrMailboxNotFound
}

func TestClaimMailboxActivatesPaidPendingMailbox(t *testing.T) {
	now := time.Now().UTC()
	tag := "tva-paid"
	mailbox := &domain.Mailbox{
		ID:               "mbx-paid",
		BillingEmail:     "billing@example.com",
		KeyFingerprint:   "edproof:key-paid",
		Status:           domain.MailboxStatusPendingPayment,
		PaymentSessionID: tag,
		PaymentURL:       "https://opencollective.com/checkout?tags=" + tag,
		CreatedAt:        now.Add(-time.Hour),
	}
	repo := &claimReconcileRepo{&fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{mailbox.KeyFingerprint: mailbox},
		byStripeSession:  map[string]*domain.Mailbox{tag: mailbox},
	}}
	gateway := &claimChargeGateway{charges: []ports.PaymentCharge{{Tag: tag, ChargeID: "tx1", ChargedAt: now.Add(-time.Minute)}}}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, gateway, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	got, created, err := svc.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{Fingerprint: mailbox.KeyFingerprint, Algorithm: "ed25519"}, "")
	if err != nil {
		t.Fatalf("ClaimMailbox failed: %v", err)
	}
	if created || !got.Usable() || got.Status != domain.MailboxStatusActive || got.ExpiresAt == nil || !got.ExpiresAt.After(now) {
		t.Fatalf("expected active, unexpired existing mailbox, got %+v, created=%v", got, created)
	}
	if gateway.calls != 1 {
		t.Fatalf("expected one paid-charge lookup, got %d", gateway.calls)
	}
}

func TestClaimReconcileHonoursCooldown(t *testing.T) {
	mailbox := &domain.Mailbox{
		ID:               "mbx-pending",
		BillingEmail:     "billing@example.com",
		KeyFingerprint:   "edproof:key-pending",
		Status:           domain.MailboxStatusPendingPayment,
		PaymentSessionID: "tva-pending",
		PaymentURL:       "https://opencollective.com/checkout?tags=tva-pending",
		CreatedAt:        time.Now().UTC().Add(-time.Hour),
	}
	repo := &claimReconcileRepo{&fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{mailbox.KeyFingerprint: mailbox},
		byStripeSession:  map[string]*domain.Mailbox{mailbox.PaymentSessionID: mailbox},
	}}
	gateway := &claimChargeGateway{}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, gateway, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	for i := 0; i < 2; i++ {
		got, created, err := svc.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{Fingerprint: mailbox.KeyFingerprint, Algorithm: "ed25519"}, "")
		if err != nil || created || got.Status != domain.MailboxStatusPendingPayment {
			t.Fatalf("claim %d: mailbox=%+v, created=%v, err=%v", i+1, got, created, err)
		}
	}
	if gateway.calls != 1 {
		t.Fatalf("expected one paid-charge lookup during cooldown, got %d", gateway.calls)
	}
}
