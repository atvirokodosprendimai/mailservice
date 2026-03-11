-- +goose Up
-- Drop the original inline UNIQUE constraint on stripe_session_id by recreating
-- the table. SQLite does not support DROP CONSTRAINT, so we use the rename-copy
-- pattern. This is the first use of this pattern in this codebase.

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

-- Step 4: Recreate all non-inline indexes
CREATE INDEX IF NOT EXISTS idx_mailboxes_status ON mailboxes(status);
CREATE INDEX IF NOT EXISTS idx_mailboxes_account_id ON mailboxes(account_id);
CREATE INDEX IF NOT EXISTS idx_mailboxes_expires_at ON mailboxes(expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mailboxes_key_fingerprint
    ON mailboxes(key_fingerprint)
    WHERE key_fingerprint IS NOT NULL AND key_fingerprint <> '';

-- Step 5: Partial unique index — only enforce uniqueness for real session IDs
CREATE UNIQUE INDEX IF NOT EXISTS idx_mailboxes_stripe_session_id
    ON mailboxes(stripe_session_id)
    WHERE stripe_session_id IS NOT NULL AND stripe_session_id <> '';

-- +goose Down
-- WARNING: This is a DESTRUCTIVE down migration. It removes the partial unique
-- index but does NOT restore the original inline UNIQUE constraint.
-- After rolling back, stripe_session_id has NO uniqueness enforcement.
DROP INDEX IF EXISTS idx_mailboxes_stripe_session_id;
