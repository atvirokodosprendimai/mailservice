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

// No t.Parallel() — OpenAndMigrate calls goose.SetBaseFS/SetDialect (global state).
func TestMailboxRepositoryRejectsDuplicateBillingEmailForKeyBoundMailboxes(t *testing.T) {
	db, err := database.OpenAndMigrate(filepath.Join(t.TempDir(), "mailboxes.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}

	repo := NewMailboxRepository(db)

	// First key-bound mailbox with billing email
	first := &domain.Mailbox{
		ID:             "mbx-first",
		OwnerEmail:     "shared@example.com",
		BillingEmail:   "shared@example.com",
		KeyFingerprint: "edproof:key-1",
		IMAPHost:       "imap.example.com",
		IMAPPort:       143,
		IMAPUsername:    "mbx_first",
		IMAPPassword:   "secret",
		AccessToken:    "access-first",
		Status:         domain.MailboxStatusActive,
	}
	if err := repo.Create(context.Background(), first); err != nil {
		t.Fatalf("Create first mailbox failed: %v", err)
	}

	// Second key-bound mailbox with same billing email should fail
	second := &domain.Mailbox{
		ID:             "mbx-second",
		OwnerEmail:     "shared@example.com",
		BillingEmail:   "shared@example.com",
		KeyFingerprint: "edproof:key-2",
		IMAPHost:       "imap.example.com",
		IMAPPort:       143,
		IMAPUsername:    "mbx_second",
		IMAPPassword:   "secret",
		AccessToken:    "access-second",
		Status:         domain.MailboxStatusPendingPayment,
	}
	if err := repo.Create(context.Background(), second); err == nil {
		t.Fatalf("expected unique constraint violation for duplicate billing email, got nil")
	}
}

// No t.Parallel() — OpenAndMigrate calls goose.SetBaseFS/SetDialect (global state).
func TestMailboxRepositoryAllowsSameBillingEmailForAccountBoundMailboxes(t *testing.T) {
	db, err := database.OpenAndMigrate(filepath.Join(t.TempDir(), "mailboxes.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}

	repo := NewMailboxRepository(db)

	// Account-bound mailboxes with same billing email should be allowed
	for i, id := range []string{"mbx-acct-a", "mbx-acct-b"} {
		mailbox := &domain.Mailbox{
			ID:           id,
			AccountID:    "acc-sponsor",
			OwnerEmail:   "sponsor@example.com",
			BillingEmail: "sponsor@example.com",
			IMAPHost:     "imap.example.com",
			IMAPPort:     143,
			IMAPUsername: fmt.Sprintf("mbx_acct_%d", i),
			IMAPPassword: "secret",
			AccessToken:  fmt.Sprintf("access-acct-%d", i),
			Status:       domain.MailboxStatusActive,
		}
		if err := repo.Create(context.Background(), mailbox); err != nil {
			t.Fatalf("Create account-bound mailbox %s failed: %v", id, err)
		}
	}
}

// No t.Parallel() — OpenAndMigrate calls goose.SetBaseFS/SetDialect (global state).
func TestMailboxRepositoryGetActiveOrPendingByBillingEmailIgnoresAccountBound(t *testing.T) {
	db, err := database.OpenAndMigrate(filepath.Join(t.TempDir(), "mailboxes.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}

	repo := NewMailboxRepository(db)

	// Create account-bound mailbox with a billing email
	acctMailbox := &domain.Mailbox{
		ID:           "mbx-acct",
		AccountID:    "acc-1",
		OwnerEmail:   "shared@example.com",
		BillingEmail: "shared@example.com",
		IMAPHost:     "imap.example.com",
		IMAPPort:     143,
		IMAPUsername: "mbx_acct",
		IMAPPassword: "secret",
		AccessToken:  "access-acct",
		Status:       domain.MailboxStatusActive,
	}
	if err := repo.Create(context.Background(), acctMailbox); err != nil {
		t.Fatalf("Create account-bound mailbox failed: %v", err)
	}

	// Query should NOT find account-bound mailbox
	_, err = repo.GetActiveOrPendingByBillingEmail(context.Background(), "shared@example.com")
	if err == nil {
		t.Fatalf("expected not found for account-bound mailbox, got nil error")
	}
}

// No t.Parallel() — OpenAndMigrate calls goose.SetBaseFS/SetDialect (global state).
func TestMailboxRepositoryGetActiveOrPendingByBillingEmail(t *testing.T) {
	db, err := database.OpenAndMigrate(filepath.Join(t.TempDir(), "mailboxes.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}

	repo := NewMailboxRepository(db)

	mailbox := &domain.Mailbox{
		ID:             "mbx-lookup",
		OwnerEmail:     "lookup@example.com",
		BillingEmail:   "lookup@example.com",
		KeyFingerprint: "edproof:key-lookup",
		IMAPHost:       "imap.example.com",
		IMAPPort:       143,
		IMAPUsername:    "mbx_lookup",
		IMAPPassword:   "secret",
		AccessToken:    "access-lookup",
		Status:         domain.MailboxStatusActive,
	}
	if err := repo.Create(context.Background(), mailbox); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := repo.GetActiveOrPendingByBillingEmail(context.Background(), "lookup@example.com")
	if err != nil {
		t.Fatalf("GetActiveOrPendingByBillingEmail failed: %v", err)
	}
	if found.ID != "mbx-lookup" {
		t.Fatalf("expected mailbox mbx-lookup, got %q", found.ID)
	}

	// Expired mailbox should not be found
	mailbox.Status = domain.MailboxStatusExpired
	if err := repo.Update(context.Background(), mailbox); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	_, err = repo.GetActiveOrPendingByBillingEmail(context.Background(), "lookup@example.com")
	if err == nil {
		t.Fatalf("expected not found for expired mailbox, got nil error")
	}
}
