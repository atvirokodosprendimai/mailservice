-- +goose Up
CREATE TABLE IF NOT EXISTS mailboxes (
    id TEXT PRIMARY KEY,
    owner_email TEXT NOT NULL,
    imap_host TEXT NOT NULL,
    imap_port INTEGER NOT NULL,
    imap_username TEXT NOT NULL UNIQUE,
    imap_password TEXT NOT NULL,
    access_token TEXT NOT NULL UNIQUE,
    stripe_session_id TEXT NOT NULL UNIQUE,
    payment_url TEXT NOT NULL,
    status TEXT NOT NULL,
    paid_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_mailboxes_status ON mailboxes(status);

-- +goose Down
DROP TABLE IF EXISTS mailboxes;
