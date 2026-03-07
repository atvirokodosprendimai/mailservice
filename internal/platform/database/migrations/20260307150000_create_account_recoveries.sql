-- +goose Up
CREATE TABLE IF NOT EXISTS account_recoveries (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL,
    code_hash TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_account_recoveries_account_id ON account_recoveries(account_id);
CREATE INDEX IF NOT EXISTS idx_account_recoveries_active ON account_recoveries(account_id, used_at);

-- +goose Down
DROP TABLE IF EXISTS account_recoveries;
