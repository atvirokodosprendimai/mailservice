-- +goose Up
ALTER TABLE mailboxes ADD COLUMN granted_months INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE mailboxes DROP COLUMN granted_months;
