-- +goose Up
ALTER TABLE organizations
    ADD COLUMN frozen_at TIMESTAMPTZ NULL;

-- +goose Down
ALTER TABLE organizations
    DROP COLUMN frozen_at;
