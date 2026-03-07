-- +goose Up
ALTER TABLE mailboxes ADD COLUMN expires_at TIMESTAMP NULL;
CREATE INDEX IF NOT EXISTS idx_mailboxes_expires_at ON mailboxes(expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_mailboxes_expires_at;
