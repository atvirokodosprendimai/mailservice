-- +goose Up
-- Rename stripe_session_id to payment_session_id: a metadata-only
-- ALTER TABLE ... RENAME COLUMN on SQLite/libSQL, not a table rebuild
-- (unlike the 20260311220000 precedent, which rebuilt the table to drop
-- an inline UNIQUE constraint — a different problem this rename doesn't have).
ALTER TABLE mailboxes RENAME COLUMN stripe_session_id TO payment_session_id;

ALTER TABLE mailboxes ADD COLUMN payment_provider TEXT NOT NULL DEFAULT 'paddle';
ALTER TABLE mailboxes ADD COLUMN subscription_id TEXT;
ALTER TABLE mailboxes ADD COLUMN last_payment_event_at TIMESTAMP;
ALTER TABLE mailboxes ADD COLUMN last_payment_event_id TEXT;

-- Backfill payment_provider for pre-existing rows by session-ID prefix, not by
-- non-emptiness: Stripe checkout session IDs start with 'cs_', Polar's do not.
UPDATE mailboxes
SET payment_provider = CASE
    WHEN payment_session_id LIKE 'cs\_%' ESCAPE '\' THEN 'stripe'
    WHEN payment_session_id IS NOT NULL AND payment_session_id <> '' THEN 'polar'
END
WHERE payment_session_id IS NOT NULL AND payment_session_id <> '';

-- Recreate the partial unique index against the renamed column.
DROP INDEX IF EXISTS idx_mailboxes_stripe_session_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_mailboxes_payment_session_id
    ON mailboxes(payment_session_id)
    WHERE payment_session_id IS NOT NULL AND payment_session_id <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_mailboxes_payment_session_id;

ALTER TABLE mailboxes DROP COLUMN last_payment_event_id;
ALTER TABLE mailboxes DROP COLUMN last_payment_event_at;
ALTER TABLE mailboxes DROP COLUMN subscription_id;
ALTER TABLE mailboxes DROP COLUMN payment_provider;

ALTER TABLE mailboxes RENAME COLUMN payment_session_id TO stripe_session_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mailboxes_stripe_session_id
    ON mailboxes(stripe_session_id)
    WHERE stripe_session_id IS NOT NULL AND stripe_session_id <> '';
