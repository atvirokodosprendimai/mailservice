package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type mailboxModel struct {
	ID               string `gorm:"primaryKey;type:text"`
	AccountID        string `gorm:"not null;index"`
	OwnerEmail       string `gorm:"not null"`
	BillingEmail     string `gorm:"not null"`
	KeyFingerprint   string
	IMAPHost         string `gorm:"not null"`
	IMAPPort         int    `gorm:"not null"`
	IMAPUsername     string `gorm:"not null;uniqueIndex"`
	IMAPPassword     string `gorm:"not null"`
	AccessToken      string `gorm:"not null;uniqueIndex"`
	PaymentSessionID string `gorm:"column:stripe_session_id;not null"`
	PaymentURL       string `gorm:"not null"`
	Status           string `gorm:"not null;index"`
	GrantedMonths    int    `gorm:"not null;default:0"`
	CouponUsed       bool   `gorm:"not null;default:false"`
	PaidAt           *time.Time
	ExpiresAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (mailboxModel) TableName() string {
	return "mailboxes"
}

func toDomain(model *mailboxModel) *domain.Mailbox {
	return &domain.Mailbox{
		ID:               model.ID,
		AccountID:        model.AccountID,
		OwnerEmail:       model.OwnerEmail,
		BillingEmail:     firstNonEmpty(model.BillingEmail, model.OwnerEmail),
		KeyFingerprint:   strings.TrimSpace(strings.ToLower(model.KeyFingerprint)),
		IMAPHost:         model.IMAPHost,
		IMAPPort:         model.IMAPPort,
		IMAPUsername:     model.IMAPUsername,
		IMAPPassword:     model.IMAPPassword,
		AccessToken:      model.AccessToken,
		PaymentSessionID: model.PaymentSessionID,
		PaymentURL:       model.PaymentURL,
		Status:           domain.MailboxStatus(model.Status),
		GrantedMonths:    model.GrantedMonths,
		CouponUsed:       model.CouponUsed,
		PaidAt:           model.PaidAt,
		ExpiresAt:        model.ExpiresAt,
		CreatedAt:        model.CreatedAt,
		UpdatedAt:        model.UpdatedAt,
	}
}

func toModel(mailbox *domain.Mailbox) *mailboxModel {
	billingEmail := firstNonEmpty(
		strings.TrimSpace(strings.ToLower(mailbox.BillingEmail)),
		strings.TrimSpace(strings.ToLower(mailbox.OwnerEmail)),
	)
	return &mailboxModel{
		ID:               mailbox.ID,
		AccountID:        mailbox.AccountID,
		OwnerEmail:       mailbox.OwnerEmail,
		BillingEmail:     billingEmail,
		KeyFingerprint:   strings.TrimSpace(strings.ToLower(mailbox.KeyFingerprint)),
		IMAPHost:         mailbox.IMAPHost,
		IMAPPort:         mailbox.IMAPPort,
		IMAPUsername:     mailbox.IMAPUsername,
		IMAPPassword:     mailbox.IMAPPassword,
		AccessToken:      mailbox.AccessToken,
		PaymentSessionID: mailbox.PaymentSessionID,
		PaymentURL:       mailbox.PaymentURL,
		Status:           string(mailbox.Status),
		GrantedMonths:    mailbox.GrantedMonths,
		CouponUsed:       mailbox.CouponUsed,
		PaidAt:           mailbox.PaidAt,
		ExpiresAt:        mailbox.ExpiresAt,
	}
}

type MailboxRepository struct {
	db *gorm.DB
}

func NewMailboxRepository(db *gorm.DB) *MailboxRepository {
	return &MailboxRepository{db: db}
}

func (r *MailboxRepository) Create(ctx context.Context, mailbox *domain.Mailbox) error {
	return r.db.WithContext(ctx).Create(toModel(mailbox)).Error
}

func (r *MailboxRepository) Update(ctx context.Context, mailbox *domain.Mailbox) error {
	return r.db.WithContext(ctx).Save(toModel(mailbox)).Error
}

func (r *MailboxRepository) GetByID(ctx context.Context, id string) (*domain.Mailbox, error) {
	var model mailboxModel
	err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrMailboxNotFound
		}
		return nil, err
	}
	return toDomain(&model), nil
}

func (r *MailboxRepository) ListByAccountID(ctx context.Context, accountID string) ([]domain.Mailbox, error) {
	var models []mailboxModel
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&models, "account_id = ?", accountID).Error
	if err != nil {
		return nil, err
	}

	items := make([]domain.Mailbox, 0, len(models))
	for i := range models {
		items = append(items, *toDomain(&models[i]))
	}
	return items, nil
}

func (r *MailboxRepository) GetPendingByAccountID(ctx context.Context, accountID string) (*domain.Mailbox, error) {
	var model mailboxModel
	err := r.db.WithContext(ctx).
		First(&model, "account_id = ? AND status = ?", accountID, string(domain.MailboxStatusPendingPayment)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrMailboxNotFound
		}
		return nil, err
	}
	return toDomain(&model), nil
}

func (r *MailboxRepository) ListPendingPayment(ctx context.Context) ([]domain.Mailbox, error) {
	var models []mailboxModel
	err := r.db.WithContext(ctx).
		Where("status = ?", string(domain.MailboxStatusPendingPayment)).
		Order("created_at ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	items := make([]domain.Mailbox, 0, len(models))
	for i := range models {
		items = append(items, *toDomain(&models[i]))
	}
	return items, nil
}

func (r *MailboxRepository) GetByPaymentSessionID(ctx context.Context, sessionID string) (*domain.Mailbox, error) {
	var model mailboxModel
	err := r.db.WithContext(ctx).First(&model, "stripe_session_id = ?", sessionID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrMailboxNotFound
		}
		return nil, err
	}
	return toDomain(&model), nil
}

func (r *MailboxRepository) GetByAccessToken(ctx context.Context, accessToken string) (*domain.Mailbox, error) {
	var model mailboxModel
	err := r.db.WithContext(ctx).First(&model, "access_token = ?", accessToken).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrMailboxNotFound
		}
		return nil, err
	}
	return toDomain(&model), nil
}

func (r *MailboxRepository) ListActiveExpired(ctx context.Context, now time.Time) ([]domain.Mailbox, error) {
	var models []mailboxModel
	err := r.db.WithContext(ctx).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at <= ?", string(domain.MailboxStatusActive), now).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	items := make([]domain.Mailbox, 0, len(models))
	for i := range models {
		items = append(items, *toDomain(&models[i]))
	}
	return items, nil
}

func (r *MailboxRepository) GetActiveOrPendingByBillingEmail(ctx context.Context, billingEmail string) (*domain.Mailbox, error) {
	var model mailboxModel
	err := r.db.WithContext(ctx).
		First(&model, "billing_email = ? AND (account_id IS NULL OR account_id = '') AND status IN (?, ?)",
			strings.TrimSpace(strings.ToLower(billingEmail)),
			string(domain.MailboxStatusActive),
			string(domain.MailboxStatusPendingPayment)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrMailboxNotFound
		}
		return nil, err
	}
	return toDomain(&model), nil
}

func (r *MailboxRepository) GetByKeyFingerprint(ctx context.Context, keyFingerprint string) (*domain.Mailbox, error) {
	var model mailboxModel
	err := r.db.WithContext(ctx).First(&model, "key_fingerprint = ?", strings.TrimSpace(strings.ToLower(keyFingerprint))).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrMailboxNotFound
		}
		return nil, err
	}
	return toDomain(&model), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
