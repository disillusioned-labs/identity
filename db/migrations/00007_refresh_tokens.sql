-- +goose Up
CREATE TABLE refresh_tokens (
    id              UUID         PRIMARY KEY DEFAULT uuidv7(),
    user_id         UUID         NOT NULL REFERENCES users(id),
    token_hash      VARCHAR(255) NOT NULL,
    user_agent      VARCHAR(500) NULL,
    ip_address      INET         NULL,
    expires_at      TIMESTAMPTZ  NOT NULL,
    revoked_at      TIMESTAMPTZ  NULL,
    last_used_at    TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_refresh_tokens_hash
    ON refresh_tokens (token_hash);

CREATE INDEX ix_refresh_tokens_user_active
    ON refresh_tokens (user_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE refresh_tokens;
