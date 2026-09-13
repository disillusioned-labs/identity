-- +goose Up
CREATE TABLE user_devices (
    id          UUID         PRIMARY KEY DEFAULT uuidv7(),
    user_id     UUID         NOT NULL REFERENCES users(id),
    token       TEXT         NOT NULL,
    platform    VARCHAR(20)  NOT NULL,
    app_version VARCHAR(50)  NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ  NULL,

    CONSTRAINT ck_user_devices_platform
        CHECK (platform IN ('android', 'ios', 'web'))
);

-- A push token is globally unique among active devices: the same token may
-- re-register after a reinstall (upsert), and may belong to a new user after
-- an app logout, so uniqueness is enforced only on the active set.
CREATE UNIQUE INDEX ux_user_devices_token_active
    ON user_devices (token) WHERE revoked_at IS NULL;

CREATE INDEX ix_user_devices_user
    ON user_devices (user_id) WHERE revoked_at IS NULL;

CREATE TRIGGER trg_user_devices_updated_at
    BEFORE UPDATE ON user_devices
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE user_devices;
