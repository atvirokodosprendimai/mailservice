package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/domain"
	"github.com/atvirokodosprendimai/mailservice/internal/platform/database"
)

// No t.Parallel() — OpenAndMigrate calls goose.SetBaseFS/SetDialect (global state).
func TestAccountRepositoryClearSubscriptionExpiresAt(t *testing.T) {
	db, err := database.OpenAndMigrate(filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}
	repo := NewAccountRepository(db)

	future := time.Now().UTC().Add(30 * 24 * time.Hour)
	subscribed := &domain.Account{
		ID:                    "acc-1",
		OwnerEmail:            "subscribed@example.com",
		APIToken:              "token-1",
		SubscriptionExpiresAt: &future,
	}
	free := &domain.Account{
		ID:         "acc-2",
		OwnerEmail: "free@example.com",
		APIToken:   "token-2",
	}
	for _, acc := range []*domain.Account{subscribed, free} {
		if err := repo.Create(context.Background(), acc); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	cleared, err := repo.ClearSubscriptionExpiresAt(context.Background())
	if err != nil {
		t.Fatalf("ClearSubscriptionExpiresAt failed: %v", err)
	}
	if cleared != 1 {
		t.Fatalf("expected 1 account cleared, got %d", cleared)
	}

	got, err := repo.GetByID(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.SubscriptionExpiresAt != nil {
		t.Fatalf("expected nil subscription expiry after clear, got %v", got.SubscriptionExpiresAt)
	}
}
