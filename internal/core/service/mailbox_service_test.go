package service

import (
	"context"
	"testing"
	"time"

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
	provisioner := &fakeMailRuntimeProvisioner{}
	accounts := &fakeMailboxAccountRepo{}
	service := NewMailboxService(repo, accounts, payment, notifier, fakeMailboxTokenGenerator{token: "token"}, provisioner)

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

func TestCreateMailboxActiveSubscriptionSkipsPaymentAndProvisioned(t *testing.T) {
	now := time.Now().UTC().Add(24 * time.Hour)
	repo := &fakeMailboxRepo{}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	provisioner := &fakeMailRuntimeProvisioner{}
	accounts := &fakeMailboxAccountRepo{}
	service := NewMailboxService(repo, accounts, payment, notifier, fakeMailboxTokenGenerator{token: "token"}, provisioner)

	mailbox, created, err := service.CreateMailbox(context.Background(), CreateMailboxRequest{
		Account: &domain.Account{ID: "acc-1", OwnerEmail: "owner@example.com", SubscriptionExpiresAt: &now},
	})
	if err != nil {
		t.Fatalf("CreateMailbox failed: %v", err)
	}
	if !created {
		t.Fatalf("expected mailbox to be newly created")
	}
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected active mailbox for subscribed account, got %s", mailbox.Status)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no payment link creation, got %d", payment.calls)
	}
	if notifier.calls != 0 {
		t.Fatalf("expected no payment notification, got %d", notifier.calls)
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected one runtime provision, got %d", provisioner.calls)
	}
}

func TestMarkMailboxPaidEnsuresRuntimeMailbox(t *testing.T) {
	repo := &fakeMailboxRepo{
		byStripeSession: map[string]*domain.Mailbox{
			"sess-1": {
				ID:              "mbx-1",
				AccountID:       "acc-1",
				IMAPUsername:    "mbx_abc",
				IMAPPassword:    "pass",
				StripeSessionID: "sess-1",
				Status:          domain.MailboxStatusPendingPayment,
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{
		byID: map[string]*domain.Account{
			"acc-1": {ID: "acc-1"},
		},
	}
	provisioner := &fakeMailRuntimeProvisioner{}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, provisioner)

	mailbox, err := service.MarkMailboxPaid(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("MarkMailboxPaid failed: %v", err)
	}
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected active status, got %s", mailbox.Status)
	}
	if mailbox.ExpiresAt == nil {
		t.Fatalf("expected expires_at to be set")
	}
	if accounts.lastSubscriptionUpdateAccountID != "acc-1" {
		t.Fatalf("expected account subscription update")
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected one runtime provisioning call, got %d", provisioner.calls)
	}
}

func TestResolveIMAPRejectsExpiredMailbox(t *testing.T) {
	expiredAt := time.Now().UTC().Add(-time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-1": {
				ID:          "mbx-1",
				AccountID:   "acc-1",
				Status:      domain.MailboxStatusActive,
				PaidAt:      ptrTime(time.Now().UTC().Add(-2 * time.Hour)),
				ExpiresAt:   &expiredAt,
				AccessToken: "token-1",
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{
		byID: map[string]*domain.Account{
			"acc-1": {ID: "acc-1", SubscriptionExpiresAt: ptrTime(time.Now().UTC().Add(-time.Minute))},
		},
	}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{})

	_, err := service.ResolveIMAPByToken(context.Background(), "token-1")
	if err != ports.ErrMailboxNotUsable {
		t.Fatalf("expected ErrMailboxNotUsable, got %v", err)
	}
}

func TestResolveIMAPAllowsPendingMailboxWhenAccountSubscribed(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-1": {
				ID:           "mbx-1",
				AccountID:    "acc-1",
				Status:       domain.MailboxStatusPendingPayment,
				AccessToken:  "token-1",
				IMAPHost:     "imap",
				IMAPPort:     143,
				IMAPUsername: "u",
				IMAPPassword: "p",
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{byID: map[string]*domain.Account{"acc-1": {ID: "acc-1", SubscriptionExpiresAt: &future}}}
	provisioner := &fakeMailRuntimeProvisioner{}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, provisioner)

	result, err := service.ResolveIMAPByToken(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("ResolveIMAPByToken failed: %v", err)
	}
	if result.Username != "u" {
		t.Fatalf("expected IMAP username u, got %s", result.Username)
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected provisioner called once")
	}
}

type fakeMailboxRepo struct {
	pendingByAccount map[string]*domain.Mailbox
	created          []*domain.Mailbox
	byStripeSession  map[string]*domain.Mailbox
	byAccessToken    map[string]*domain.Mailbox
}

type fakeMailboxAccountRepo struct {
	byID                            map[string]*domain.Account
	lastSubscriptionUpdateAccountID string
	lastSubscriptionUpdateExpiresAt time.Time
}

func (f *fakeMailboxAccountRepo) Create(_ context.Context, _ *domain.Account) error { return nil }

func (f *fakeMailboxAccountRepo) GetByID(_ context.Context, accountID string) (*domain.Account, error) {
	if f.byID != nil {
		if item, ok := f.byID[accountID]; ok {
			return item, nil
		}
	}
	return nil, ports.ErrAccountNotFound
}

func (f *fakeMailboxAccountRepo) GetByOwnerEmail(_ context.Context, _ string) (*domain.Account, error) {
	return nil, ports.ErrAccountNotFound
}

func (f *fakeMailboxAccountRepo) GetByAPIToken(_ context.Context, _ string) (*domain.Account, error) {
	return nil, ports.ErrAccountNotFound
}

func (f *fakeMailboxAccountRepo) UpdateAPIToken(_ context.Context, _ string, _ string) error {
	return nil
}

func (f *fakeMailboxAccountRepo) UpdateSubscriptionExpiresAt(_ context.Context, accountID string, expiresAt time.Time) error {
	f.lastSubscriptionUpdateAccountID = accountID
	f.lastSubscriptionUpdateExpiresAt = expiresAt
	if f.byID == nil {
		f.byID = map[string]*domain.Account{}
	}
	if item, ok := f.byID[accountID]; ok {
		item.SubscriptionExpiresAt = &expiresAt
	}
	return nil
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

func (f *fakeMailboxRepo) GetByStripeSessionID(_ context.Context, sessionID string) (*domain.Mailbox, error) {
	if f.byStripeSession != nil {
		if item, ok := f.byStripeSession[sessionID]; ok {
			return item, nil
		}
	}
	return nil, ports.ErrMailboxNotFound
}

func (f *fakeMailboxRepo) GetByAccessToken(_ context.Context, accessToken string) (*domain.Mailbox, error) {
	if f.byAccessToken != nil {
		if item, ok := f.byAccessToken[accessToken]; ok {
			return item, nil
		}
	}
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

type fakeMailRuntimeProvisioner struct {
	calls int
}

func (f *fakeMailRuntimeProvisioner) EnsureMailbox(_ context.Context, _ *domain.Mailbox) error {
	f.calls++
	return nil
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func (f *fakeMailboxNotifier) SendPaymentLink(_ context.Context, _ string, _ string, _ string) error {
	f.calls++
	return nil
}

func (f *fakeMailboxNotifier) SendRecoveryLink(_ context.Context, _ string, _ string) error {
	return nil
}
