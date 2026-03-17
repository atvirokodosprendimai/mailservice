package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type supportMessageModel struct {
	ID             string    `gorm:"primaryKey;type:text"`
	MailboxID      string    `gorm:"not null"`
	KeyFingerprint string    `gorm:"not null;index:idx_support_messages_fingerprint_created"`
	Subject        string    `gorm:"not null"`
	Body           string    `gorm:"not null"`
	CreatedAt      time.Time `gorm:"not null;index:idx_support_messages_fingerprint_created"`
}

func (supportMessageModel) TableName() string {
	return "support_messages"
}

func toSupportMessageModel(msg *domain.SupportMessage) *supportMessageModel {
	return &supportMessageModel{
		ID:             msg.ID,
		MailboxID:      msg.MailboxID,
		KeyFingerprint: msg.KeyFingerprint,
		Subject:        msg.Subject,
		Body:           msg.Body,
		CreatedAt:      msg.CreatedAt,
	}
}

type SupportMessageRepository struct {
	db *gorm.DB
}

func NewSupportMessageRepository(db *gorm.DB) *SupportMessageRepository {
	return &SupportMessageRepository{db: db}
}

func (r *SupportMessageRepository) Create(ctx context.Context, msg *domain.SupportMessage) error {
	return r.db.WithContext(ctx).Create(toSupportMessageModel(msg)).Error
}

func (r *SupportMessageRepository) CountRecentByFingerprint(ctx context.Context, fingerprint string, since time.Time) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&supportMessageModel{}).
		Where("key_fingerprint = ? AND created_at >= ?", fingerprint, since).
		Count(&count).Error
	return int(count), err
}
