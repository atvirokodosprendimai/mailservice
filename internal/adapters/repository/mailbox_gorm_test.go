package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/domain"
	"github.com/atvirokodosprendimai/mailservice/internal/platform/database"
)

func TestMailboxRepositoryPersistsBillingEmailAndKeyFingerprint(t *testing.T) {
	t.Parallel()

	db, err := database.OpenAndMigrate(filepath.Join(t.TempDir(), "mailboxes.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}

	repo := NewMailboxRepository(db)
	mailbox := &domain.Mailbox{
		ID:               "mbx-1",
		AccountID:        "acc-1",
		OwnerEmail:       "legacy@example.com",
		BillingEmail:     "billing@example.com",
		KeyFingerprint:   "edproof:abc123",
		IMAPHost:         "imap.example.com",
		IMAPPort:         143,
		IMAPUsername:     "mbx_abc123",
		IMAPPassword:     "secret",
		AccessToken:      "access-1",
		PaymentSessionID: "payment-1",
		PaymentURL:       "https://pay.example.com/session/1",
		Status:           domain.MailboxStatusPendingPayment,
	}

	if err := repo.Create(context.Background(), mailbox); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	stored, err := repo.GetByID(context.Background(), mailbox.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.BillingEmail != "billing@example.com" {
		t.Fatalf("expected billing email, got %q", stored.BillingEmail)
	}
	if stored.KeyFingerprint != "edproof:abc123" {
		t.Fatalf("expected key fingerprint, got %q", stored.KeyFingerprint)
	}

	byKey, err := repo.GetByKeyFingerprint(context.Background(), "edproof:abc123")
	if err != nil {
		t.Fatalf("GetByKeyFingerprint failed: %v", err)
	}
	if byKey.ID != mailbox.ID {
		t.Fatalf("expected mailbox id %q, got %q", mailbox.ID, byKey.ID)
	}
}

func TestMailboxRepositoryFallsBackBillingEmailToOwnerEmail(t *testing.T) {
	t.Parallel()

	db, err := database.OpenAndMigrate(filepath.Join(t.TempDir(), "mailboxes.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}

	repo := NewMailboxRepository(db)
	mailbox := &domain.Mailbox{
		ID:               "mbx-2",
		AccountID:        "acc-1",
		OwnerEmail:       "owner@example.com",
		IMAPHost:         "imap.example.com",
		IMAPPort:         143,
		IMAPUsername:     "mbx_owner",
		IMAPPassword:     "secret",
		AccessToken:      "access-2",
		PaymentSessionID: "payment-2",
		PaymentURL:       "https://pay.example.com/session/2",
		Status:           domain.MailboxStatusPendingPayment,
	}

	if err := repo.Create(context.Background(), mailbox); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	stored, err := repo.GetByID(context.Background(), mailbox.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.BillingEmail != "owner@example.com" {
		t.Fatalf("expected billing email fallback, got %q", stored.BillingEmail)
	}
}
