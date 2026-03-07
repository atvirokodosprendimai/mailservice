-- +goose Up
ALTER TABLE accounts ADD COLUMN subscription_expires_at TIMESTAMP NULL;
CREATE INDEX IF NOT EXISTS idx_accounts_subscription_expires_at ON accounts(subscription_expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_accounts_subscription_expires_at;
