package repository

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type MailRuntimeProvisioner struct {
	db         *gorm.DB
	mailDomain string
}

func NewMailRuntimeProvisioner(db *gorm.DB, mailDomain string) *MailRuntimeProvisioner {
	return &MailRuntimeProvisioner{db: db, mailDomain: strings.TrimSpace(strings.ToLower(mailDomain))}
}

func (p *MailRuntimeProvisioner) EnsureMailbox(ctx context.Context, mailbox *domain.Mailbox) error {
	domain := p.mailDomain
	if domain == "" {
		domain = "mail.local"
	}

	address := mailbox.IMAPUsername + "@" + domain
	maildir := domain + "/" + mailbox.IMAPUsername

	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT OR IGNORE INTO mail_domains(domain) VALUES(?)", domain).Error; err != nil {
			return fmt.Errorf("ensure mail domain: %w", err)
		}

		query := "INSERT INTO mail_users(login,email,password,maildir,enabled,updated_at) VALUES(?,?,?,?,1,CURRENT_TIMESTAMP) " +
			"ON CONFLICT(login) DO UPDATE SET email=excluded.email,password=excluded.password,maildir=excluded.maildir,enabled=1,updated_at=CURRENT_TIMESTAMP"
		if err := tx.Exec(query, mailbox.IMAPUsername, address, mailbox.IMAPPassword, maildir).Error; err != nil {
			return fmt.Errorf("ensure mail user: %w", err)
		}

		return nil
	})
}
