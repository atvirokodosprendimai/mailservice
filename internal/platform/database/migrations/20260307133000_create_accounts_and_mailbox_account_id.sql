-- +goose Up
CREATE TABLE IF NOT EXISTS accounts (
    id TEXT PRIMARY KEY,
    owner_email TEXT NOT NULL UNIQUE,
    api_token TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE mailboxes ADD COLUMN account_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_mailboxes_account_id ON mailboxes(account_id);

-- +goose Down
DROP INDEX IF EXISTS idx_mailboxes_account_id;
DROP TABLE IF EXISTS accounts;
