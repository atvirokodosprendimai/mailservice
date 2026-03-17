-- +goose Up
CREATE TABLE IF NOT EXISTS support_messages (
    id TEXT PRIMARY KEY,
    mailbox_id TEXT NOT NULL,
    key_fingerprint TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_support_messages_fingerprint_created
    ON support_messages(key_fingerprint, created_at);

-- +goose Down
DROP TABLE IF EXISTS support_messages;
