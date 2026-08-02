-- +goose Up
CREATE TABLE signing_keys (
    kid                   VARCHAR(50) PRIMARY KEY,
    private_key_encrypted BYTEA       NOT NULL,
    public_key            TEXT        NOT NULL,
    algorithm             VARCHAR(10) NOT NULL DEFAULT 'RS256',
    is_active             BOOLEAN     NOT NULL DEFAULT false,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    retired_at            TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX ux_signing_keys_active
    ON signing_keys ((true)) WHERE is_active = true;

-- +goose Down
DROP TABLE signing_keys;
