-- +goose Up
-- Enforce one billing email per active/pending key-bound mailbox at the DB level.
-- Only applies to key-bound mailboxes (account_id is empty) to avoid blocking
-- the legacy account-based flow where multiple mailboxes share the account email.
CREATE UNIQUE INDEX IF NOT EXISTS idx_mailboxes_billing_email_active
    ON mailboxes(billing_email)
    WHERE billing_email <> ''
    AND (account_id IS NULL OR account_id = '')
    AND status IN ('active', 'pending_payment');

-- +goose Down
DROP INDEX IF EXISTS idx_mailboxes_billing_email_active;
