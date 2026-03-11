---
title: "fix: Sponsor cannot create more than one mailbox"
type: fix
status: completed
date: 2026-03-11
---

# fix: Sponsor cannot create more than one mailbox

## Overview

When an account (sponsor) with an active subscription calls `POST /v1/mailboxes` a second time, the INSERT fails with a **unique constraint violation** on `stripe_session_id`.
The first sponsored mailbox succeeds because `""` is unique; the second one collides on the same empty string.

## Problem Statement

The original `CREATE TABLE mailboxes` migration defines:

```sql
stripe_session_id TEXT NOT NULL UNIQUE
payment_url TEXT NOT NULL
```

When `MailboxService.CreateMailbox` creates a mailbox for an account with an active subscription, it skips payment entirely — `PaymentSessionID` and `PaymentURL` stay as Go zero values (`""`).
The GORM model persists these as empty strings.

- **First mailbox:** `stripe_session_id = ""` → inserted fine (unique)
- **Second mailbox:** `stripe_session_id = ""` → **UNIQUE constraint failed**

The `key_fingerprint` column already has the correct pattern:
```sql
CREATE UNIQUE INDEX idx_mailboxes_key_fingerprint
    ON mailboxes(key_fingerprint)
    WHERE key_fingerprint IS NOT NULL AND key_fingerprint <> '';
```

The `stripe_session_id` needs the same treatment.

## Root Cause

- `internal/platform/database/migrations/20260307120000_create_mailboxes.sql:10` — `stripe_session_id TEXT NOT NULL UNIQUE`
- `internal/adapters/repository/mailbox_gorm.go:26` — `PaymentSessionID string \`gorm:"column:stripe_session_id;not null;uniqueIndex"\``
- `internal/core/service/mailbox_service.go:187-202` — sponsored mailbox created without `PaymentSessionID`

## Proposed Solution

1. **New migration**: drop the inline `UNIQUE` on `stripe_session_id`, add a partial unique index that excludes empty values
2. **Update GORM model**: remove `uniqueIndex` tag, allow empty values (no schema change needed for `not null` — empty string satisfies it)
3. **Test**: add a service-level test that creates two mailboxes for the same sponsored account

## Acceptance Criteria

- [x] An account with an active subscription can create 2+ mailboxes without error
- [x] Unique constraint still prevents duplicate non-empty `stripe_session_id` values
- [x] Existing migration is not modified (new migration only)
- [x] Service-level test `TestCreateMailboxMultipleForSponsoredAccount` passes
- [x] Integration test against real SQLite DB confirms no constraint violation
- [x] All existing tests still pass

## MVP

### Phase 1: Migration

**`internal/platform/database/migrations/20260311220000_fix_stripe_session_id_unique_constraint.sql`**

```sql
-- +goose Up
-- Drop the original UNIQUE constraint by recreating the table (SQLite limitation).
-- SQLite doesn't support DROP CONSTRAINT, so we use the rename-copy pattern.

-- Step 1: Create new table without the inline UNIQUE on stripe_session_id
CREATE TABLE mailboxes_new (
    id TEXT PRIMARY KEY,
    owner_email TEXT NOT NULL,
    billing_email TEXT NOT NULL DEFAULT '',
    key_fingerprint TEXT NULL,
    imap_host TEXT NOT NULL,
    imap_port INTEGER NOT NULL,
    imap_username TEXT NOT NULL UNIQUE,
    imap_password TEXT NOT NULL,
    access_token TEXT NOT NULL UNIQUE,
    stripe_session_id TEXT NOT NULL DEFAULT '',
    payment_url TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    paid_at TIMESTAMP NULL,
    expires_at TIMESTAMP NULL,
    account_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Step 2: Copy data (explicit target columns for defensive correctness)
INSERT INTO mailboxes_new (
    id, owner_email, billing_email, key_fingerprint,
    imap_host, imap_port, imap_username, imap_password,
    access_token, stripe_session_id, payment_url,
    status, paid_at, expires_at, account_id,
    created_at, updated_at
) SELECT
    id, owner_email, billing_email, key_fingerprint,
    imap_host, imap_port, imap_username, imap_password,
    access_token, stripe_session_id, payment_url,
    status, paid_at, expires_at, account_id,
    created_at, updated_at
FROM mailboxes;

-- Step 3: Swap tables
DROP TABLE mailboxes;
ALTER TABLE mailboxes_new RENAME TO mailboxes;

-- Step 4: Recreate all indexes
CREATE INDEX IF NOT EXISTS idx_mailboxes_status ON mailboxes(status);
CREATE INDEX IF NOT EXISTS idx_mailboxes_account_id ON mailboxes(account_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mailboxes_key_fingerprint
    ON mailboxes(key_fingerprint)
    WHERE key_fingerprint IS NOT NULL AND key_fingerprint <> '';

-- Step 5: Partial unique index — only enforce uniqueness for real session IDs
CREATE UNIQUE INDEX IF NOT EXISTS idx_mailboxes_stripe_session_id
    ON mailboxes(stripe_session_id)
    WHERE stripe_session_id IS NOT NULL AND stripe_session_id <> '';

-- Step 6: Recreate expires_at index (from migration 20260307200000, lost during table drop)
CREATE INDEX IF NOT EXISTS idx_mailboxes_expires_at ON mailboxes(expires_at);

-- +goose Down
-- WARNING: This is a DESTRUCTIVE down migration. It removes the partial unique
-- index but does NOT restore the original inline UNIQUE constraint.
-- After rolling back, stripe_session_id has NO uniqueness enforcement.
-- To fully reverse, a new rename-copy migration restoring inline UNIQUE is needed
-- (only safe if no duplicate empty-string values exist).
DROP INDEX IF EXISTS idx_mailboxes_stripe_session_id;
```

### Phase 2: GORM model update

**`internal/adapters/repository/mailbox_gorm.go:26`**

```go
// Before:
PaymentSessionID string `gorm:"column:stripe_session_id;not null;uniqueIndex"`

// After:
PaymentSessionID string `gorm:"column:stripe_session_id;not null"`
```

### Phase 3: Service test

**`internal/core/service/mailbox_service_test.go`** — add:

```go
func TestCreateMailboxMultipleForSponsoredAccount(t *testing.T) {
    now := time.Now().UTC().Add(24 * time.Hour)
    repo := &fakeMailboxRepo{}
    payment := &fakePaymentGateway{}
    provisioner := &fakeMailRuntimeProvisioner{}
    svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, provisioner, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

    account := &domain.Account{ID: "acc-1", OwnerEmail: "sponsor@example.com", SubscriptionExpiresAt: &now}

    first, created1, err := svc.CreateMailbox(context.Background(), CreateMailboxRequest{Account: account})
    if err != nil {
        t.Fatalf("first CreateMailbox failed: %v", err)
    }
    if !created1 {
        t.Fatalf("expected first mailbox to be newly created")
    }

    second, created2, err := svc.CreateMailbox(context.Background(), CreateMailboxRequest{Account: account})
    if err != nil {
        t.Fatalf("second CreateMailbox failed: %v", err)
    }
    if !created2 {
        t.Fatalf("expected second mailbox to be newly created")
    }

    if first.ID == second.ID {
        t.Fatalf("expected different mailbox IDs, both are %q", first.ID)
    }
    if payment.calls != 0 {
        t.Fatalf("expected no payment link creation, got %d", payment.calls)
    }
    if provisioner.calls != 2 {
        t.Fatalf("expected two provisions, got %d", provisioner.calls)
    }
}
```

### Phase 4: Integration test (repository level)

**`internal/adapters/repository/mailbox_gorm_test.go`** — add:

```go
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
```

## System-Wide Impact

- **Payment webhook safety**: `GetByPaymentSessionID` still works — it queries by a specific non-empty session ID. The partial index covers this.
- **No API surface change**: `POST /v1/mailboxes` behavior unchanged for callers, it just stops failing on the second call.
- **ReprovisionMailbox also benefits**: `ReprovisionMailbox` (line 551 of `mailbox_service.go`) also creates mailboxes with empty `PaymentSessionID` — this fix unblocks that path too.
- **Migration risk**: The SQLite rename-copy pattern is standard for SQLite constraint changes (first use in this codebase). Goose wraps migrations in a transaction for atomicity. Data is preserved.

## Review Findings (from technical review)

### Addressed in this plan
- 🔴 **P1**: Missing `idx_mailboxes_expires_at` recreation — **FIXED** (added Step 6)
- 🟡 **P2**: INSERT without explicit target column list — **FIXED** (added target columns)
- 🟡 **P2**: Plan incorrectly claimed rename-copy was "well-tested in codebase" — **FIXED** (corrected text)
- 🟡 **P2**: Down migration warning too vague — **FIXED** (added explicit WARNING comment)

### Deferred (separate scope)
- 🟡 **P2**: Unbounded mailbox creation per sponsor — add per-account mailbox cap in `CreateMailbox`. Not a regression from this fix, but this fix removes an accidental safety net. Track as separate issue.
- 🔵 **P3**: Guard `GetByPaymentSessionID` against empty string queries — defense-in-depth, no real-world call path passes empty strings today.
- 🔵 **P3**: Negative integration test verifying non-empty duplicate session IDs still fail — confirms partial index works in both directions.

### Implementation notes from review
- The service-level test proves logic (fakes don't enforce constraints); the integration test proves the actual DB fix.
- Integration test needs `"fmt"` import and `// No t.Parallel()` comment for pattern consistency.

## Sources

- Root cause: [`internal/platform/database/migrations/20260307120000_create_mailboxes.sql:10`](internal/platform/database/migrations/20260307120000_create_mailboxes.sql)
- Affected service method: [`internal/core/service/mailbox_service.go:159`](internal/core/service/mailbox_service.go) (`CreateMailbox`)
- GORM model: [`internal/adapters/repository/mailbox_gorm.go:26`](internal/adapters/repository/mailbox_gorm.go)
- Existing partial index pattern: [`internal/platform/database/migrations/20260308095500_add_mailbox_key_fingerprint_and_billing_email.sql:6-8`](internal/platform/database/migrations/20260308095500_add_mailbox_key_fingerprint_and_billing_email.sql)
