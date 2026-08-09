package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
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
func TestMailboxRepositoryActivationToken(t *testing.T) {
	db, err := database.OpenAndMigrate(filepath.Join(t.TempDir(), "mailboxes.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}

	repo := NewMailboxRepository(db)
	expiresAt := time.Now().Add(24 * time.Hour).UTC()
	mailbox := &domain.Mailbox{
		ID:                  "mbx-act-1",
		AccountID:           "acc-1",
		OwnerEmail:          "owner@example.com",
		BillingEmail:        "billing@example.com",
		KeyFingerprint:      "edproof:act1",
		IMAPHost:            "imap.example.com",
		IMAPPort:            143,
		IMAPUsername:        "mbx_act_1",
		IMAPPassword:        "secret",
		AccessToken:         "access-act-1",
		PaymentSessionID:    "",
		PaymentURL:          "",
		ActivationTokenHash: "hash-act-1",
		ActivationExpiresAt: &expiresAt,
		Status:              domain.MailboxStatusPendingPayment,
	}

	if err := repo.Create(context.Background(), mailbox); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	byHash, err := repo.GetByActivationTokenHash(context.Background(), "hash-act-1")
	if err != nil {
		t.Fatalf("GetByActivationTokenHash failed: %v", err)
	}
	if byHash.ID != mailbox.ID {
		t.Fatalf("expected mailbox id %q, got %q", mailbox.ID, byHash.ID)
	}
	if byHash.ActivationTokenHash != "hash-act-1" {
		t.Fatalf("expected activation token hash, got %q", byHash.ActivationTokenHash)
	}
	if byHash.ActivationExpiresAt == nil || !byHash.ActivationExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected activation expiry %v, got %v", expiresAt, byHash.ActivationExpiresAt)
	}

	if _, err := repo.GetByActivationTokenHash(context.Background(), "hash-unknown"); !errors.Is(err, ports.ErrMailboxNotFound) {
		t.Fatalf("expected ErrMailboxNotFound for unknown hash, got %v", err)
	}
}

func TestMailboxRepositoryListActiveAndClearActiveExpiries(t *testing.T) {
	db, err := database.OpenAndMigrate(filepath.Join(t.TempDir(), "mailboxes.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}
	repo := NewMailboxRepository(db)

	future := time.Now().UTC().Add(24 * time.Hour)
	active := &domain.Mailbox{
		ID:             "mbx-active",
		OwnerEmail:     "active@example.com",
		BillingEmail:   "active@example.com",
		KeyFingerprint: "edproof:active",
		IMAPHost:       "imap.example.com",
		IMAPPort:       143,
		IMAPUsername:   "mbx_active",
		IMAPPassword:   "secret",
		AccessToken:    "access-active",
		Status:         domain.MailboxStatusActive,
		PaidAt:         func() *time.Time { t := time.Now().UTC().Add(-time.Hour); return &t }(),
		ExpiresAt:      &future,
	}
	pending := &domain.Mailbox{
		ID:             "mbx-pending",
		OwnerEmail:     "pending@example.com",
		BillingEmail:   "pending@example.com",
		KeyFingerprint: "edproof:pending",
		IMAPHost:       "imap.example.com",
		IMAPPort:       143,
		IMAPUsername:   "mbx_pending",
		IMAPPassword:   "secret",
		AccessToken:    "access-pending",
		Status:         domain.MailboxStatusPendingPayment,
	}
	for _, mb := range []*domain.Mailbox{active, pending} {
		if err := repo.Create(context.Background(), mb); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	list, err := repo.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != "mbx-active" {
		t.Fatalf("expected only the active mailbox, got %+v", list)
	}

	cleared, err := repo.ClearActiveExpiries(context.Background())
	if err != nil {
		t.Fatalf("ClearActiveExpiries failed: %v", err)
	}
	if cleared != 1 {
		t.Fatalf("expected 1 row cleared, got %d", cleared)
	}

	got, err := repo.GetByID(context.Background(), "mbx-active")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.ExpiresAt != nil {
		t.Fatalf("expected nil ExpiresAt after clear, got %v", got.ExpiresAt)
	}
	if got.Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox to stay active, got %s", got.Status)
	}
}
