package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

const recoveryTTL = 10 * time.Minute

type AccountService struct {
	accounts   ports.AccountRepository
	recoveries ports.AccountRecoveryRepository
	notifier   ports.Notifier
	tokenGen   ports.TokenGenerator
}

func NewAccountService(accounts ports.AccountRepository, recoveries ports.AccountRecoveryRepository, notifier ports.Notifier, tokenGen ports.TokenGenerator) *AccountService {
	return &AccountService{accounts: accounts, recoveries: recoveries, notifier: notifier, tokenGen: tokenGen}
}

func (s *AccountService) CreateAccount(ctx context.Context, ownerEmail string) (*domain.Account, error) {
	ownerEmail = normalizeEmail(ownerEmail)
	if ownerEmail == "" || !strings.Contains(ownerEmail, "@") {
		return nil, errors.New("owner_email must be a valid email")
	}

	_, err := s.accounts.GetByOwnerEmail(ctx, ownerEmail)
	if err == nil {
		return nil, ports.ErrAccountExists
	}
	if !errors.Is(err, ports.ErrAccountNotFound) {
		return nil, err
	}

	token, err := s.tokenGen.NewToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate api token: %w", err)
	}

	account := &domain.Account{
		ID:         uuid.NewString(),
		OwnerEmail: ownerEmail,
		APIToken:   token,
	}
	if err := s.accounts.Create(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

func (s *AccountService) StartRecovery(ctx context.Context, ownerEmail string) error {
	ownerEmail = normalizeEmail(ownerEmail)
	if ownerEmail == "" || !strings.Contains(ownerEmail, "@") {
		return nil
	}

	account, err := s.accounts.GetByOwnerEmail(ctx, ownerEmail)
	if err != nil {
		if errors.Is(err, ports.ErrAccountNotFound) {
			return nil
		}
		return err
	}

	code, err := s.tokenGen.NewToken(12)
	if err != nil {
		return fmt.Errorf("generate recovery code: %w", err)
	}

	now := time.Now().UTC()
	recovery := &domain.AccountRecovery{
		ID:        uuid.NewString(),
		AccountID: account.ID,
		CodeHash:  hashToken(code),
		ExpiresAt: now.Add(recoveryTTL),
	}

	if err := s.recoveries.DeleteActiveByAccountID(ctx, account.ID); err != nil {
		return err
	}
	if err := s.recoveries.Create(ctx, recovery); err != nil {
		return err
	}

	if err := s.notifier.SendRecoveryCode(ctx, ownerEmail, code); err != nil {
		return err
	}

	return nil
}

func (s *AccountService) CompleteRecovery(ctx context.Context, ownerEmail string, code string) (*domain.Account, error) {
	ownerEmail = normalizeEmail(ownerEmail)
	code = strings.TrimSpace(code)
	if ownerEmail == "" || code == "" {
		return nil, ports.ErrRecoveryInvalid
	}

	account, err := s.accounts.GetByOwnerEmail(ctx, ownerEmail)
	if err != nil {
		if errors.Is(err, ports.ErrAccountNotFound) {
			return nil, ports.ErrRecoveryInvalid
		}
		return nil, err
	}

	recovery, err := s.recoveries.GetLatestByAccountID(ctx, account.ID)
	if err != nil {
		if errors.Is(err, ports.ErrRecoveryNotFound) {
			return nil, ports.ErrRecoveryInvalid
		}
		return nil, err
	}

	now := time.Now().UTC()
	if recovery.ExpiresAt.Before(now) {
		return nil, ports.ErrRecoveryExpired
	}

	if !tokenHashEqual(recovery.CodeHash, hashToken(code)) {
		return nil, ports.ErrRecoveryInvalid
	}

	if err := s.recoveries.MarkUsed(ctx, recovery.ID, now); err != nil {
		return nil, err
	}

	newToken, err := s.tokenGen.NewToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate api token: %w", err)
	}
	if err := s.accounts.UpdateAPIToken(ctx, account.ID, newToken); err != nil {
		return nil, err
	}

	account.APIToken = newToken
	return account, nil
}

func (s *AccountService) GetByToken(ctx context.Context, apiToken string) (*domain.Account, error) {
	apiToken = strings.TrimSpace(apiToken)
	if apiToken == "" {
		return nil, ports.ErrAccountNotFound
	}
	return s.accounts.GetByAPIToken(ctx, apiToken)
}

func normalizeEmail(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func tokenHashEqual(a string, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
