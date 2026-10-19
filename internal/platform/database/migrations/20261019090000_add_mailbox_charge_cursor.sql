-- +goose Up
ALTER TABLE mailboxes ADD COLUMN last_payment_charge_id TEXT NOT NULL DEFAULT '';
ALTER TABLE mailboxes ADD COLUMN last_payment_charged_at TIMESTAMP NULL;

-- +goose Down
ALTER TABLE mailboxes DROP COLUMN last_payment_charged_at;
ALTER TABLE mailboxes DROP COLUMN last_payment_charge_id;
