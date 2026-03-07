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

func (accountModel) TableName() string {
	return "accounts"
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
