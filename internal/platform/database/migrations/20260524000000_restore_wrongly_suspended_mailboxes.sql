-- +goose Up
-- Recovery for 20260414120000_add_billing_email_unique_constraint.sql:
-- its HAVING COUNT(*) > 1 subquery caused unique active/pending key-bound
-- mailboxes to be suspended. Restore one legitimate winner per billing email.
-- Filter: skip emails that already own an active/pending_payment row.
-- Otherwise restoring a suspended row would violate
-- idx_mailboxes_billing_email_active (one active/pending row per email).
WITH ranked_recovery_candidates AS (
    SELECT
        rowid AS mailbox_rowid,
        ROW_NUMBER() OVER (
            PARTITION BY billing_email
            ORDER BY
                CASE WHEN paid_at IS NOT NULL THEN 0 ELSE 1 END,
                rowid DESC
        ) AS recovery_rank
    FROM mailboxes
    WHERE status = 'suspended'
      AND billing_email <> ''
      AND (account_id IS NULL OR account_id = '')
      AND billing_email NOT IN (
          SELECT billing_email
          FROM mailboxes
          WHERE status IN ('active', 'pending_payment')
            AND billing_email <> ''
            AND (account_id IS NULL OR account_id = '')
      )
)
UPDATE mailboxes
SET
    status = CASE
        WHEN paid_at IS NULL THEN 'pending_payment'
        WHEN expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP THEN 'active'
        ELSE 'expired'
    END,
    updated_at = CURRENT_TIMESTAMP
WHERE rowid IN (
    SELECT mailbox_rowid
    FROM ranked_recovery_candidates
    WHERE recovery_rank = 1
);

-- +goose Down
-- Recovery is one-way; re-suspending would only restore broken state.
SELECT 1;
