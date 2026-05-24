-- +goose Up
-- 20260414120000 added idx_mailboxes_billing_email_active and a buggy
-- UPDATE that suspended key-bound active/pending mailboxes. Both moves
-- encoded a false business rule: one owner (billing_email) may legitimately
-- hold multiple subscriptions / mailboxes. Drop the index, then restore
-- every still-suspended key-bound row to its real status based on its
-- own paid_at and expires_at. Recovery is per-row; no per-email winner
-- selection is needed because uniqueness was never the correct invariant.
--
-- Supersedes 20260524000000_restore_wrongly_suspended_mailboxes.sql, which
-- ran as a no-op once data drift left every suspended email with a
-- coexisting pending_payment sibling that its NOT IN filter excluded.

DROP INDEX IF EXISTS idx_mailboxes_billing_email_active;

UPDATE mailboxes
SET
    status = CASE
        WHEN paid_at IS NULL THEN 'pending_payment'
        WHEN expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP THEN 'active'
        ELSE 'expired'
    END,
    updated_at = CURRENT_TIMESTAMP
WHERE status = 'suspended'
  AND billing_email <> ''
  AND (account_id IS NULL OR account_id = '');

-- +goose Down
-- Recovery is one-way; the dropped unique index encoded an incorrect
-- product rule and restoring rows to suspended would only mask data.
SELECT 1;
