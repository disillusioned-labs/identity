-- +goose Up
CREATE TABLE password_recovery_tokens (
    user_id     UUID         PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    token_hash  VARCHAR(64)  NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ  NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT chk_password_recovery_token_hash
        CHECK (token_hash ~ '^[0-9a-f]{64}$')
);

CREATE INDEX ix_password_recovery_tokens_expiry
    ON password_recovery_tokens (expires_at);

-- +goose Down
DROP TABLE password_recovery_tokens;
