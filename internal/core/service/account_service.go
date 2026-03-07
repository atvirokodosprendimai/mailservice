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
const recoveryRateLimitWindow = 1 * time.Minute
const refreshTokenTTL = 30 * 24 * time.Hour

type AuthTokens struct {
	APIToken     string
	RefreshToken string
}

type AccountService struct {
	accounts      ports.AccountRepository
	recoveries    ports.AccountRecoveryRepository
	refreshTokens ports.RefreshTokenRepository
	notifier      ports.Notifier
	tokenGen      ports.TokenGenerator
}

func NewAccountService(accounts ports.AccountRepository, recoveries ports.AccountRecoveryRepository, refreshTokens ports.RefreshTokenRepository, notifier ports.Notifier, tokenGen ports.TokenGenerator) *AccountService {
	return &AccountService{
		accounts:      accounts,
		recoveries:    recoveries,
		refreshTokens: refreshTokens,
		notifier:      notifier,
		tokenGen:      tokenGen,
	}
}

func (s *AccountService) CreateAccount(ctx context.Context, ownerEmail string) (*domain.Account, *AuthTokens, error) {
	ownerEmail = normalizeEmail(ownerEmail)
	if ownerEmail == "" || !strings.Contains(ownerEmail, "@") {
		return nil, nil, errors.New("owner_email must be a valid email")
	}

	_, err := s.accounts.GetByOwnerEmail(ctx, ownerEmail)
	if err == nil {
		return nil, nil, ports.ErrAccountExists
	}
	if !errors.Is(err, ports.ErrAccountNotFound) {
		return nil, nil, err
	}

	apiToken, err := s.tokenGen.NewToken(32)
	if err != nil {
		return nil, nil, fmt.Errorf("generate api token: %w", err)
	}

	account := &domain.Account{
		ID:         uuid.NewString(),
		OwnerEmail: ownerEmail,
		APIToken:   apiToken,
	}
	if err := s.accounts.Create(ctx, account); err != nil {
		return nil, nil, err
	}

	refreshToken, err := s.issueRefreshToken(ctx, account.ID, time.Now().UTC())
	if err != nil {
		return nil, nil, err
	}

	return account, &AuthTokens{APIToken: apiToken, RefreshToken: refreshToken}, nil
}

func (s *AccountService) RefreshAccess(ctx context.Context, refreshToken string) (*domain.Account, *AuthTokens, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, nil, ports.ErrRefreshNotFound
	}

	now := time.Now().UTC()
	stored, err := s.refreshTokens.GetActiveByTokenHash(ctx, hashToken(refreshToken))
	if err != nil {
		if errors.Is(err, ports.ErrRefreshNotFound) {
			return nil, nil, ports.ErrRefreshNotFound
		}
		return nil, nil, err
	}

	if stored.ExpiresAt.Before(now) {
		return nil, nil, ports.ErrRefreshExpired
	}

	if err := s.refreshTokens.MarkUsed(ctx, stored.ID, now); err != nil {
		return nil, nil, err
	}

	newAPIToken, err := s.tokenGen.NewToken(32)
	if err != nil {
		return nil, nil, fmt.Errorf("generate api token: %w", err)
	}
	if err := s.accounts.UpdateAPIToken(ctx, stored.AccountID, newAPIToken); err != nil {
		return nil, nil, err
	}

	newRefreshToken, err := s.issueRefreshToken(ctx, stored.AccountID, now)
	if err != nil {
		return nil, nil, err
	}

	account, err := s.accounts.GetByAPIToken(ctx, newAPIToken)
	if err != nil {
		return nil, nil, err
	}

	return account, &AuthTokens{APIToken: newAPIToken, RefreshToken: newRefreshToken}, nil
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

	now := time.Now().UTC()
	if err := s.enforceRecoveryRateLimit(ctx, account.ID, now); err != nil {
		return err
	}

	return s.createAndSendRecoveryCode(ctx, account, ownerEmail, now)
}

func (s *AccountService) CompleteRecovery(ctx context.Context, ownerEmail string, code string) (*domain.Account, *AuthTokens, error) {
	ownerEmail = normalizeEmail(ownerEmail)
	code = strings.TrimSpace(code)
	if ownerEmail == "" || code == "" {
		return nil, nil, ports.ErrRecoveryInvalid
	}

	account, err := s.accounts.GetByOwnerEmail(ctx, ownerEmail)
	if err != nil {
		if errors.Is(err, ports.ErrAccountNotFound) {
			return nil, nil, ports.ErrRecoveryInvalid
		}
		return nil, nil, err
	}

	recovery, err := s.recoveries.GetLatestActiveByAccountID(ctx, account.ID)
	if err != nil {
		if errors.Is(err, ports.ErrRecoveryNotFound) {
			return nil, nil, ports.ErrRecoveryInvalid
		}
		return nil, nil, err
	}

	now := time.Now().UTC()
	if recovery.ExpiresAt.Before(now) {
		return nil, nil, ports.ErrRecoveryExpired
	}

	if !tokenHashEqual(recovery.CodeHash, hashToken(code)) {
		return nil, nil, ports.ErrRecoveryInvalid
	}

	if err := s.recoveries.MarkUsed(ctx, recovery.ID, now); err != nil {
		return nil, nil, err
	}

	newAPIToken, err := s.tokenGen.NewToken(32)
	if err != nil {
		return nil, nil, fmt.Errorf("generate api token: %w", err)
	}
	if err := s.accounts.UpdateAPIToken(ctx, account.ID, newAPIToken); err != nil {
		return nil, nil, err
	}

	newRefreshToken, err := s.issueRefreshToken(ctx, account.ID, now)
	if err != nil {
		return nil, nil, err
	}

	account.APIToken = newAPIToken
	return account, &AuthTokens{APIToken: newAPIToken, RefreshToken: newRefreshToken}, nil
}

func (s *AccountService) GetByToken(ctx context.Context, apiToken string) (*domain.Account, error) {
	apiToken = strings.TrimSpace(apiToken)
	if apiToken == "" {
		return nil, ports.ErrAccountNotFound
	}
	return s.accounts.GetByAPIToken(ctx, apiToken)
}

func (s *AccountService) enforceRecoveryRateLimit(ctx context.Context, accountID string, now time.Time) error {
	latest, err := s.recoveries.GetLatestByAccountID(ctx, accountID)
	if err != nil {
		if errors.Is(err, ports.ErrRecoveryNotFound) {
			return nil
		}
		return err
	}

	if now.Sub(latest.CreatedAt) < recoveryRateLimitWindow {
		return ports.ErrRateLimitReached
	}
	return nil
}

func (s *AccountService) createAndSendRecoveryCode(ctx context.Context, account *domain.Account, ownerEmail string, now time.Time) error {
	code, err := s.tokenGen.NewToken(12)
	if err != nil {
		return fmt.Errorf("generate recovery code: %w", err)
	}

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

func (s *AccountService) issueRefreshToken(ctx context.Context, accountID string, now time.Time) (string, error) {
	rawToken, err := s.tokenGen.NewToken(48)
	if err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}

	stored := &domain.RefreshToken{
		ID:        uuid.NewString(),
		AccountID: accountID,
		TokenHash: hashToken(rawToken),
		ExpiresAt: now.Add(refreshTokenTTL),
	}
	if err := s.refreshTokens.Create(ctx, stored); err != nil {
		return "", err
	}

	return rawToken, nil
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
