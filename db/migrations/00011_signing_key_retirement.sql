-- +goose Up
-- Graduated signing-key rotation: a deactivated key stays published in JWKS
-- (is_active = false, retired_at still NULL) until the retirement step marks
-- it retired after a grace period >= the access-token TTL, so tokens signed
-- just before rotation keep verifying until they expire.
ALTER TABLE signing_keys
    ADD COLUMN deactivated_at TIMESTAMPTZ NULL;

-- +goose Down
ALTER TABLE signing_keys
    DROP COLUMN deactivated_at;
