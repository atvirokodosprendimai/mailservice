package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type AccountService struct {
	repo     ports.AccountRepository
	tokenGen ports.TokenGenerator
}

func NewAccountService(repo ports.AccountRepository, tokenGen ports.TokenGenerator) *AccountService {
	return &AccountService{repo: repo, tokenGen: tokenGen}
}

func (s *AccountService) CreateOrGetAccount(ctx context.Context, ownerEmail string) (*domain.Account, bool, error) {
	ownerEmail = strings.TrimSpace(strings.ToLower(ownerEmail))
	if ownerEmail == "" || !strings.Contains(ownerEmail, "@") {
		return nil, false, errors.New("owner_email must be a valid email")
	}

	existing, err := s.repo.GetByOwnerEmail(ctx, ownerEmail)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, ports.ErrAccountNotFound) {
		return nil, false, err
	}

	token, err := s.tokenGen.NewToken(32)
	if err != nil {
		return nil, false, fmt.Errorf("generate api token: %w", err)
	}

	account := &domain.Account{
		ID:         uuid.NewString(),
		OwnerEmail: ownerEmail,
		APIToken:   token,
	}
	if err := s.repo.Create(ctx, account); err != nil {
		return nil, false, err
	}

	return account, true, nil
}

func (s *AccountService) GetByToken(ctx context.Context, apiToken string) (*domain.Account, error) {
	apiToken = strings.TrimSpace(apiToken)
	if apiToken == "" {
		return nil, ports.ErrAccountNotFound
	}
	return s.repo.GetByAPIToken(ctx, apiToken)
}
