package service

import (
	"context"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

func TestCreateAccountFailsWhenAlreadyExists(t *testing.T) {
	accounts := &fakeAccountRepo{
		byOwner: map[string]*domain.Account{
			"owner@example.com": {ID: "acc-1", OwnerEmail: "owner@example.com", APIToken: "token-1"},
		},
	}
	service := NewAccountService(accounts, &fakeRecoveryRepo{}, &fakeAccountNotifier{}, &fakeTokenGenerator{tokens: []string{"new-token"}})

	_, err := service.CreateAccount(context.Background(), "owner@example.com")
	if err == nil {
		t.Fatalf("expected account exists error")
	}
	if err != ports.ErrAccountExists {
		t.Fatalf("expected ErrAccountExists, got %v", err)
	}
}

func TestStartRecoveryRateLimited(t *testing.T) {
	accounts := &fakeAccountRepo{
		byOwner: map[string]*domain.Account{
			"owner@example.com": {ID: "acc-1", OwnerEmail: "owner@example.com", APIToken: "token-1"},
		},
	}
	recoveries := &fakeRecoveryRepo{
		latest: &domain.AccountRecovery{
			ID:        "rec-last",
			AccountID: "acc-1",
			CreatedAt: time.Now().UTC(),
		},
	}
	service := NewAccountService(accounts, recoveries, &fakeAccountNotifier{}, &fakeTokenGenerator{tokens: []string{"recover-code"}})

	err := service.StartRecovery(context.Background(), "owner@example.com")
	if err == nil {
		t.Fatalf("expected rate limit error")
	}
	if err != ports.ErrRateLimitReached {
		t.Fatalf("expected ErrRateLimitReached, got %v", err)
	}
}

func TestStartRecoveryCreatesCodeAndSendsNotification(t *testing.T) {
	accounts := &fakeAccountRepo{
		byOwner: map[string]*domain.Account{
			"owner@example.com": {ID: "acc-1", OwnerEmail: "owner@example.com", APIToken: "token-1"},
		},
	}
	recoveries := &fakeRecoveryRepo{}
	notifier := &fakeAccountNotifier{}
	service := NewAccountService(accounts, recoveries, notifier, &fakeTokenGenerator{tokens: []string{"recover-code"}})

	err := service.StartRecovery(context.Background(), "owner@example.com")
	if err != nil {
		t.Fatalf("StartRecovery failed: %v", err)
	}
	if notifier.recoveryCalls != 1 {
		t.Fatalf("expected one recovery notification, got %d", notifier.recoveryCalls)
	}
	if notifier.lastRecoveryCode != "recover-code" {
		t.Fatalf("expected recovery code to be sent")
	}
	if recoveries.latest == nil {
		t.Fatalf("expected recovery record created")
	}
}

func TestStartAccountAccessCreatesAccountWithoutEnumeration(t *testing.T) {
	accounts := &fakeAccountRepo{}
	recoveries := &fakeRecoveryRepo{}
	notifier := &fakeAccountNotifier{}
	service := NewAccountService(accounts, recoveries, notifier, &fakeTokenGenerator{tokens: []string{"new-api-token", "recover-code"}})

	err := service.StartAccountAccess(context.Background(), "owner@example.com")
	if err != nil {
		t.Fatalf("StartAccountAccess failed: %v", err)
	}
	if accounts.byOwner["owner@example.com"] == nil {
		t.Fatalf("expected account to be created")
	}
	if notifier.recoveryCalls != 1 {
		t.Fatalf("expected recovery code send")
	}
}

func TestCompleteRecoveryRotatesToken(t *testing.T) {
	accounts := &fakeAccountRepo{
		byOwner: map[string]*domain.Account{
			"owner@example.com": {ID: "acc-1", OwnerEmail: "owner@example.com", APIToken: "old-token"},
		},
	}
	recoveries := &fakeRecoveryRepo{
		latest: &domain.AccountRecovery{
			ID:        "rec-1",
			AccountID: "acc-1",
			CodeHash:  hashToken("recover-code"),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
		},
	}
	service := NewAccountService(accounts, recoveries, &fakeAccountNotifier{}, &fakeTokenGenerator{tokens: []string{"new-api-token"}})

	account, err := service.CompleteRecovery(context.Background(), "owner@example.com", "recover-code")
	if err != nil {
		t.Fatalf("CompleteRecovery failed: %v", err)
	}
	if account.APIToken != "new-api-token" {
		t.Fatalf("expected rotated token, got %q", account.APIToken)
	}
	if accounts.lastUpdatedToken != "new-api-token" {
		t.Fatalf("expected repository token update")
	}
	if recoveries.markedUsedID != "rec-1" {
		t.Fatalf("expected recovery marked as used")
	}
}

type fakeAccountRepo struct {
	byOwner          map[string]*domain.Account
	byToken          map[string]*domain.Account
	lastUpdatedToken string
}

func (f *fakeAccountRepo) Create(_ context.Context, account *domain.Account) error {
	if f.byOwner == nil {
		f.byOwner = map[string]*domain.Account{}
	}
	if f.byToken == nil {
		f.byToken = map[string]*domain.Account{}
	}
	f.byOwner[account.OwnerEmail] = account
	f.byToken[account.APIToken] = account
	return nil
}

func (f *fakeAccountRepo) GetByOwnerEmail(_ context.Context, ownerEmail string) (*domain.Account, error) {
	if item, ok := f.byOwner[ownerEmail]; ok {
		return item, nil
	}
	return nil, ports.ErrAccountNotFound
}

func (f *fakeAccountRepo) GetByAPIToken(_ context.Context, apiToken string) (*domain.Account, error) {
	if item, ok := f.byToken[apiToken]; ok {
		return item, nil
	}
	return nil, ports.ErrAccountNotFound
}

func (f *fakeAccountRepo) UpdateAPIToken(_ context.Context, accountID string, apiToken string) error {
	for _, item := range f.byOwner {
		if item.ID == accountID {
			item.APIToken = apiToken
			f.lastUpdatedToken = apiToken
			return nil
		}
	}
	return ports.ErrAccountNotFound
}

type fakeRecoveryRepo struct {
	latest       *domain.AccountRecovery
	markedUsedID string
}

func (f *fakeRecoveryRepo) Create(_ context.Context, recovery *domain.AccountRecovery) error {
	f.latest = recovery
	return nil
}

func (f *fakeRecoveryRepo) DeleteActiveByAccountID(_ context.Context, accountID string) error {
	if f.latest != nil && f.latest.AccountID == accountID {
		f.latest = nil
	}
	return nil
}

func (f *fakeRecoveryRepo) GetLatestByAccountID(_ context.Context, accountID string) (*domain.AccountRecovery, error) {
	if f.latest != nil && f.latest.AccountID == accountID {
		return f.latest, nil
	}
	return nil, ports.ErrRecoveryNotFound
}

func (f *fakeRecoveryRepo) GetLatestActiveByAccountID(_ context.Context, accountID string) (*domain.AccountRecovery, error) {
	if f.latest != nil && f.latest.AccountID == accountID && f.latest.UsedAt == nil {
		return f.latest, nil
	}
	return nil, ports.ErrRecoveryNotFound
}

func (f *fakeRecoveryRepo) MarkUsed(_ context.Context, recoveryID string, usedAt time.Time) error {
	f.markedUsedID = recoveryID
	if f.latest != nil && f.latest.ID == recoveryID {
		f.latest.UsedAt = &usedAt
	}
	return nil
}

type fakeTokenGenerator struct {
	tokens []string
}

func (f *fakeTokenGenerator) NewToken(_ int) (string, error) {
	if len(f.tokens) == 0 {
		return "", nil
	}
	value := f.tokens[0]
	f.tokens = f.tokens[1:]
	return value, nil
}

type fakeAccountNotifier struct {
	recoveryCalls    int
	lastRecoveryCode string
}

func (f *fakeAccountNotifier) SendPaymentLink(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

func (f *fakeAccountNotifier) SendRecoveryCode(_ context.Context, _ string, code string) error {
	f.recoveryCalls++
	f.lastRecoveryCode = code
	return nil
}
