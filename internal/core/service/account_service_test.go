package service

import (
	"context"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

func TestCreateOrGetAccountReturnsExisting(t *testing.T) {
	repo := &fakeAccountRepo{
		byOwner: map[string]*domain.Account{
			"owner@example.com": {
				ID:         "acc-1",
				OwnerEmail: "owner@example.com",
				APIToken:   "token-1",
			},
		},
	}
	service := NewAccountService(repo, fakeTokenGenerator{"new-token"})

	account, created, err := service.CreateOrGetAccount(context.Background(), "owner@example.com")
	if err != nil {
		t.Fatalf("CreateOrGetAccount failed: %v", err)
	}
	if created {
		t.Fatalf("expected existing account, got created=true")
	}
	if account.APIToken != "token-1" {
		t.Fatalf("expected existing token, got %q", account.APIToken)
	}
}

type fakeAccountRepo struct {
	byOwner map[string]*domain.Account
	byToken map[string]*domain.Account
}

type fakeTokenGenerator struct {
	token string
}

func (f fakeTokenGenerator) NewToken(_ int) (string, error) {
	return f.token, nil
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
