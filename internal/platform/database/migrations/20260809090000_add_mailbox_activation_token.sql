-- +goose Up
ALTER TABLE mailboxes ADD COLUMN activation_token_hash TEXT NULL;
ALTER TABLE mailboxes ADD COLUMN activation_expires_at TIMESTAMP NULL;
CREATE INDEX IF NOT EXISTS idx_mailboxes_activation_token_hash ON mailboxes(activation_token_hash);

-- +goose Down
DROP INDEX IF EXISTS idx_mailboxes_activation_token_hash;
ALTER TABLE mailboxes DROP COLUMN activation_token_hash;
ALTER TABLE mailboxes DROP COLUMN activation_expires_at;
