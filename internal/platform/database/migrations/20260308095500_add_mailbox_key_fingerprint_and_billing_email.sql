-- +goose Up
ALTER TABLE mailboxes ADD COLUMN billing_email TEXT NOT NULL DEFAULT '';
UPDATE mailboxes SET billing_email = owner_email WHERE billing_email = '';

ALTER TABLE mailboxes ADD COLUMN key_fingerprint TEXT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_mailboxes_key_fingerprint
    ON mailboxes(key_fingerprint)
    WHERE key_fingerprint IS NOT NULL AND key_fingerprint <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_mailboxes_key_fingerprint;
