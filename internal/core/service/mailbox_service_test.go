package service

import (
	"context"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

func TestCreateMailboxReturnsExistingPendingMailbox(t *testing.T) {
	repo := &fakeMailboxRepo{
		pendingByAccount: map[string]*domain.Mailbox{
			"acc-1": {
				ID:         "mbx-1",
				AccountID:  "acc-1",
				Status:     domain.MailboxStatusPendingPayment,
				PaymentURL: "http://pay/1",
			},
		},
	}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, payment, notifier, fakeMailboxTokenGenerator{token: "token"})

	mailbox, created, err := service.CreateMailbox(context.Background(), CreateMailboxRequest{
		Account: &domain.Account{ID: "acc-1", OwnerEmail: "owner@example.com"},
	})
	if err != nil {
		t.Fatalf("CreateMailbox failed: %v", err)
	}
	if created {
		t.Fatalf("expected pending mailbox reuse, got created=true")
	}
	if mailbox.ID != "mbx-1" {
		t.Fatalf("expected existing mailbox id, got %q", mailbox.ID)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no payment link creation, got %d", payment.calls)
	}
	if notifier.calls != 0 {
		t.Fatalf("expected no notifier call, got %d", notifier.calls)
	}
}

type fakeMailboxRepo struct {
	pendingByAccount map[string]*domain.Mailbox
	created          []*domain.Mailbox
}

func (f *fakeMailboxRepo) Create(_ context.Context, mailbox *domain.Mailbox) error {
	f.created = append(f.created, mailbox)
	return nil
}

func (f *fakeMailboxRepo) Update(_ context.Context, _ *domain.Mailbox) error {
	return nil
}

func (f *fakeMailboxRepo) GetByID(_ context.Context, _ string) (*domain.Mailbox, error) {
	return nil, ports.ErrMailboxNotFound
}

func (f *fakeMailboxRepo) ListByAccountID(_ context.Context, _ string) ([]domain.Mailbox, error) {
	return nil, nil
}

func (f *fakeMailboxRepo) GetPendingByAccountID(_ context.Context, accountID string) (*domain.Mailbox, error) {
	if item, ok := f.pendingByAccount[accountID]; ok {
		return item, nil
	}
	return nil, ports.ErrMailboxNotFound
}

func (f *fakeMailboxRepo) GetByStripeSessionID(_ context.Context, _ string) (*domain.Mailbox, error) {
	return nil, ports.ErrMailboxNotFound
}

func (f *fakeMailboxRepo) GetByAccessToken(_ context.Context, _ string) (*domain.Mailbox, error) {
	return nil, ports.ErrMailboxNotFound
}

type fakePaymentGateway struct {
	calls int
}

func (f *fakePaymentGateway) CreatePaymentLink(_ context.Context, _ ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	f.calls++
	return &ports.PaymentLink{SessionID: "sess-1", URL: "http://pay/1"}, nil
}

type fakeMailboxTokenGenerator struct {
	token string
}

func (f fakeMailboxTokenGenerator) NewToken(_ int) (string, error) {
	return f.token, nil
}

type fakeMailboxNotifier struct {
	calls int
}

func (f *fakeMailboxNotifier) SendPaymentLink(_ context.Context, _ string, _ string, _ string) error {
	f.calls++
	return nil
}

func (f *fakeMailboxNotifier) SendRecoveryCode(_ context.Context, _ string, _ string) error {
	return nil
}
