-- +goose Up
ALTER TABLE mailboxes ADD COLUMN coupon_used BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE mailboxes DROP COLUMN coupon_used;
