package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type accountModel struct {
	ID         string `gorm:"primaryKey;type:text"`
	OwnerEmail string `gorm:"not null;uniqueIndex"`
	APIToken   string `gorm:"not null;uniqueIndex"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type accountRecoveryModel struct {
	ID        string `gorm:"primaryKey;type:text"`
	AccountID string `gorm:"not null;index"`
	CodeHash  string `gorm:"not null"`
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type refreshTokenModel struct {
	ID        string `gorm:"primaryKey;type:text"`
	AccountID string `gorm:"not null;index"`
	TokenHash string `gorm:"not null;uniqueIndex"`
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (accountModel) TableName() string {
	return "accounts"
}

func (accountRecoveryModel) TableName() string {
	return "account_recoveries"
}

func (refreshTokenModel) TableName() string {
	return "refresh_tokens"
}

func toAccountDomain(m *accountModel) *domain.Account {
	return &domain.Account{
		ID:         m.ID,
		OwnerEmail: m.OwnerEmail,
		APIToken:   m.APIToken,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}

func toAccountModel(a *domain.Account) *accountModel {
	return &accountModel{
		ID:         a.ID,
		OwnerEmail: a.OwnerEmail,
		APIToken:   a.APIToken,
	}
}

func toAccountRecoveryDomain(m *accountRecoveryModel) *domain.AccountRecovery {
	return &domain.AccountRecovery{
		ID:        m.ID,
		AccountID: m.AccountID,
		CodeHash:  m.CodeHash,
		ExpiresAt: m.ExpiresAt,
		UsedAt:    m.UsedAt,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func toAccountRecoveryModel(r *domain.AccountRecovery) *accountRecoveryModel {
	return &accountRecoveryModel{
		ID:        r.ID,
		AccountID: r.AccountID,
		CodeHash:  r.CodeHash,
		ExpiresAt: r.ExpiresAt,
		UsedAt:    r.UsedAt,
	}
}

func toRefreshTokenDomain(m *refreshTokenModel) *domain.RefreshToken {
	return &domain.RefreshToken{
		ID:        m.ID,
		AccountID: m.AccountID,
		TokenHash: m.TokenHash,
		ExpiresAt: m.ExpiresAt,
		UsedAt:    m.UsedAt,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func toRefreshTokenModel(t *domain.RefreshToken) *refreshTokenModel {
	return &refreshTokenModel{
		ID:        t.ID,
		AccountID: t.AccountID,
		TokenHash: t.TokenHash,
		ExpiresAt: t.ExpiresAt,
		UsedAt:    t.UsedAt,
	}
}

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) Create(ctx context.Context, account *domain.Account) error {
	return r.db.WithContext(ctx).Create(toAccountModel(account)).Error
}

func (r *AccountRepository) GetByOwnerEmail(ctx context.Context, ownerEmail string) (*domain.Account, error) {
	var model accountModel
	err := r.db.WithContext(ctx).First(&model, "owner_email = ?", ownerEmail).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrAccountNotFound
		}
		return nil, err
	}
	return toAccountDomain(&model), nil
}

func (r *AccountRepository) GetByAPIToken(ctx context.Context, apiToken string) (*domain.Account, error) {
	var model accountModel
	err := r.db.WithContext(ctx).First(&model, "api_token = ?", apiToken).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrAccountNotFound
		}
		return nil, err
	}
	return toAccountDomain(&model), nil
}

func (r *AccountRepository) UpdateAPIToken(ctx context.Context, accountID string, apiToken string) error {
	return r.db.WithContext(ctx).Model(&accountModel{}).
		Where("id = ?", accountID).
		Updates(map[string]any{"api_token": apiToken, "updated_at": time.Now().UTC()}).Error
}

type AccountRecoveryRepository struct {
	db *gorm.DB
}

func NewAccountRecoveryRepository(db *gorm.DB) *AccountRecoveryRepository {
	return &AccountRecoveryRepository{db: db}
}

func (r *AccountRecoveryRepository) Create(ctx context.Context, recovery *domain.AccountRecovery) error {
	return r.db.WithContext(ctx).Create(toAccountRecoveryModel(recovery)).Error
}

func (r *AccountRecoveryRepository) DeleteActiveByAccountID(ctx context.Context, accountID string) error {
	return r.db.WithContext(ctx).
		Where("account_id = ? AND used_at IS NULL", accountID).
		Delete(&accountRecoveryModel{}).Error
}

func (r *AccountRecoveryRepository) GetLatestByAccountID(ctx context.Context, accountID string) (*domain.AccountRecovery, error) {
	var model accountRecoveryModel
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		First(&model, "account_id = ?", accountID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrRecoveryNotFound
		}
		return nil, err
	}
	return toAccountRecoveryDomain(&model), nil
}

func (r *AccountRecoveryRepository) GetLatestActiveByAccountID(ctx context.Context, accountID string) (*domain.AccountRecovery, error) {
	var model accountRecoveryModel
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		First(&model, "account_id = ? AND used_at IS NULL", accountID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrRecoveryNotFound
		}
		return nil, err
	}
	return toAccountRecoveryDomain(&model), nil
}

func (r *AccountRecoveryRepository) MarkUsed(ctx context.Context, recoveryID string, usedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&accountRecoveryModel{}).
		Where("id = ?", recoveryID).
		Updates(map[string]any{"used_at": usedAt.UTC(), "updated_at": usedAt.UTC()}).Error
}

type RefreshTokenRepository struct {
	db *gorm.DB
}

func NewRefreshTokenRepository(db *gorm.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, token *domain.RefreshToken) error {
	return r.db.WithContext(ctx).Create(toRefreshTokenModel(token)).Error
}

func (r *RefreshTokenRepository) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	var model refreshTokenModel
	err := r.db.WithContext(ctx).
		First(&model, "token_hash = ? AND used_at IS NULL", tokenHash).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrRefreshNotFound
		}
		return nil, err
	}
	return toRefreshTokenDomain(&model), nil
}

func (r *RefreshTokenRepository) MarkUsed(ctx context.Context, tokenID string, usedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&refreshTokenModel{}).
		Where("id = ?", tokenID).
		Updates(map[string]any{"used_at": usedAt.UTC(), "updated_at": usedAt.UTC()}).Error
}
