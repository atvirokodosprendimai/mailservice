package service

import (
	"context"
	"errors"
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
	service := NewAccountService(accounts, &fakeRecoveryRepo{}, &fakeRefreshTokenRepo{}, &fakeAccountNotifier{}, &fakeTokenGenerator{tokens: []string{"new-token"}}, "http://localhost:8080")

	_, _, err := service.CreateAccount(context.Background(), "owner@example.com")
	if err == nil {
		t.Fatalf("expected account exists error")
	}
	if !errors.Is(err, ports.ErrAccountExists) {
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
	service := NewAccountService(accounts, recoveries, &fakeRefreshTokenRepo{}, &fakeAccountNotifier{}, &fakeTokenGenerator{tokens: []string{"recover-code"}}, "http://localhost:8080")

	err := service.StartRecovery(context.Background(), "owner@example.com")
	if err == nil {
		t.Fatalf("expected rate limit error")
	}
	if !errors.Is(err, ports.ErrRateLimitReached) {
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
	service := NewAccountService(accounts, recoveries, &fakeRefreshTokenRepo{}, notifier, &fakeTokenGenerator{tokens: []string{"recover-code"}}, "http://localhost:8080")

	err := service.StartRecovery(context.Background(), "owner@example.com")
	if err != nil {
		t.Fatalf("StartRecovery failed: %v", err)
	}
	if notifier.recoveryCalls != 1 {
		t.Fatalf("expected one recovery notification, got %d", notifier.recoveryCalls)
	}
	if notifier.lastRecoveryURL == "" {
		t.Fatalf("expected recovery link to be sent")
	}
	if recoveries.latest == nil {
		t.Fatalf("expected recovery record created")
	}
}

func TestCreateAccountReturnsRefreshToken(t *testing.T) {
	accounts := &fakeAccountRepo{}
	refresh := &fakeRefreshTokenRepo{}
	service := NewAccountService(accounts, &fakeRecoveryRepo{}, refresh, &fakeAccountNotifier{}, &fakeTokenGenerator{tokens: []string{"new-api-token", "new-refresh"}}, "http://localhost:8080")

	account, tokens, err := service.CreateAccount(context.Background(), "owner@example.com")
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}
	if account.APIToken != "new-api-token" {
		t.Fatalf("expected api token, got %q", account.APIToken)
	}
	if tokens.RefreshToken != "new-refresh" {
		t.Fatalf("expected refresh token, got %q", tokens.RefreshToken)
	}
	if refresh.lastCreated == nil {
		t.Fatalf("expected refresh token to be stored")
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
	refresh := &fakeRefreshTokenRepo{}
	service := NewAccountService(accounts, recoveries, refresh, &fakeAccountNotifier{}, &fakeTokenGenerator{tokens: []string{"new-api-token", "new-refresh"}}, "http://localhost:8080")

	account, tokens, err := service.CompleteRecoveryByToken(context.Background(), "recover-code")
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
	if tokens.RefreshToken != "new-refresh" {
		t.Fatalf("expected refresh token, got %q", tokens.RefreshToken)
	}
	if refresh.lastCreated == nil {
		t.Fatalf("expected refresh token stored")
	}
}

func TestRefreshAccessRotatesTokens(t *testing.T) {
	accounts := &fakeAccountRepo{
		byOwner: map[string]*domain.Account{
			"owner@example.com": {ID: "acc-1", OwnerEmail: "owner@example.com", APIToken: "old-api"},
		},
		byToken: map[string]*domain.Account{
			"old-api": {ID: "acc-1", OwnerEmail: "owner@example.com", APIToken: "old-api"},
		},
	}
	refresh := &fakeRefreshTokenRepo{
		activeByHash: map[string]*domain.RefreshToken{
			hashToken("old-refresh"): {
				ID:        "rt-1",
				AccountID: "acc-1",
				TokenHash: hashToken("old-refresh"),
				ExpiresAt: time.Now().UTC().Add(time.Hour),
			},
		},
	}
	service := NewAccountService(accounts, &fakeRecoveryRepo{}, refresh, &fakeAccountNotifier{}, &fakeTokenGenerator{tokens: []string{"new-api", "new-refresh"}}, "http://localhost:8080")

	_, tokens, err := service.RefreshAccess(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("RefreshAccess failed: %v", err)
	}
	if accounts.lastUpdatedToken != "new-api" {
		t.Fatalf("expected api token rotation")
	}
	if refresh.markedUsedID != "rt-1" {
		t.Fatalf("expected old refresh token used")
	}
	if tokens.RefreshToken != "new-refresh" {
		t.Fatalf("expected new refresh token")
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

func (f *fakeAccountRepo) GetByID(_ context.Context, accountID string) (*domain.Account, error) {
	for _, item := range f.byOwner {
		if item.ID == accountID {
			return item, nil
		}
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
			if f.byToken == nil {
				f.byToken = map[string]*domain.Account{}
			}
			item.APIToken = apiToken
			f.byToken[apiToken] = item
			f.lastUpdatedToken = apiToken
			return nil
		}
	}
	return ports.ErrAccountNotFound
}

func (f *fakeAccountRepo) UpdateSubscriptionExpiresAt(_ context.Context, accountID string, expiresAt time.Time) error {
	for _, item := range f.byOwner {
		if item.ID == accountID {
			item.SubscriptionExpiresAt = &expiresAt
			return nil
		}
	}
	return ports.ErrAccountNotFound
}

type fakeRecoveryRepo struct {
	latest       *domain.AccountRecovery
	markedUsedID string
}

type fakeRefreshTokenRepo struct {
	activeByHash map[string]*domain.RefreshToken
	lastCreated  *domain.RefreshToken
	markedUsedID string
}

func (f *fakeRefreshTokenRepo) Create(_ context.Context, token *domain.RefreshToken) error {
	f.lastCreated = token
	if f.activeByHash == nil {
		f.activeByHash = map[string]*domain.RefreshToken{}
	}
	f.activeByHash[token.TokenHash] = token
	return nil
}

func (f *fakeRefreshTokenRepo) GetActiveByTokenHash(_ context.Context, tokenHash string) (*domain.RefreshToken, error) {
	if item, ok := f.activeByHash[tokenHash]; ok && item.UsedAt == nil {
		return item, nil
	}
	return nil, ports.ErrRefreshNotFound
}

func (f *fakeRefreshTokenRepo) MarkUsed(_ context.Context, tokenID string, usedAt time.Time) error {
	f.markedUsedID = tokenID
	for _, item := range f.activeByHash {
		if item.ID == tokenID {
			item.UsedAt = &usedAt
			break
		}
	}
	return nil
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

func (f *fakeRecoveryRepo) GetActiveByCodeHash(_ context.Context, codeHash string) (*domain.AccountRecovery, error) {
	if f.latest != nil && f.latest.CodeHash == codeHash && f.latest.UsedAt == nil {
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
	recoveryCalls   int
	lastRecoveryURL string
}

func (f *fakeAccountNotifier) SendPaymentLink(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

func (f *fakeAccountNotifier) SendRecoveryLink(_ context.Context, _ string, recoveryURL string) error {
	f.recoveryCalls++
	f.lastRecoveryURL = recoveryURL
	return nil
}

func (f *fakeAccountNotifier) SendSupportMessage(_ context.Context, _ ports.SupportMessageParams) error {
	return nil
}
