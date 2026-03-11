package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/domain"
	"github.com/atvirokodosprendimai/mailservice/internal/platform/database"
)

// No t.Parallel() — OpenAndMigrate calls goose.SetBaseFS/SetDialect (global state).
func TestMailboxRepositoryPersistsBillingEmailAndKeyFingerprint(t *testing.T) {
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

// No t.Parallel() — OpenAndMigrate calls goose.SetBaseFS/SetDialect (global state).
func TestMailboxRepositoryFallsBackBillingEmailToOwnerEmail(t *testing.T) {
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

// No t.Parallel() — OpenAndMigrate calls goose.SetBaseFS/SetDialect (global state).
func TestMailboxRepositoryAllowsMultipleEmptyPaymentSessionIDs(t *testing.T) {
	db, err := database.OpenAndMigrate(filepath.Join(t.TempDir(), "mailboxes.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}

	repo := NewMailboxRepository(db)
	for i, id := range []string{"mbx-a", "mbx-b"} {
		mailbox := &domain.Mailbox{
			ID:           id,
			AccountID:    "acc-sponsor",
			OwnerEmail:   "sponsor@example.com",
			IMAPHost:     "imap.example.com",
			IMAPPort:     143,
			IMAPUsername: fmt.Sprintf("mbx_%d", i),
			IMAPPassword: "secret",
			AccessToken:  fmt.Sprintf("access-%d", i),
			Status:       domain.MailboxStatusActive,
		}
		if err := repo.Create(context.Background(), mailbox); err != nil {
			t.Fatalf("Create mailbox %s failed: %v", id, err)
		}
	}

	mailboxes, err := repo.ListByAccountID(context.Background(), "acc-sponsor")
	if err != nil {
		t.Fatalf("ListByAccountID failed: %v", err)
	}
	if len(mailboxes) != 2 {
		t.Fatalf("expected 2 mailboxes, got %d", len(mailboxes))
	}
}
