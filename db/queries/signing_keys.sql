-- name: GetActiveSigningKey :one
SELECT kid, private_key_encrypted, public_key, algorithm
FROM signing_keys
WHERE is_active = true;

-- name: GetActiveSigningKeyAge :one
SELECT kid, created_at
FROM signing_keys
WHERE is_active = true;

-- name: ListPublishedSigningKeys :many
-- The JWKS document publishes the signing key AND keys awaiting retirement:
-- an inactive key with no retired_at was deactivated by a rotation and its
-- tokens may not have expired yet.
SELECT kid, private_key_encrypted, public_key, algorithm
FROM signing_keys
WHERE is_active = true
   OR (deactivated_at IS NOT NULL AND retired_at IS NULL);

-- name: ListActiveSigningKeys :many
SELECT kid, private_key_encrypted, public_key, algorithm
FROM signing_keys
WHERE is_active = true;

-- name: InsertSigningKey :exec
INSERT INTO signing_keys (kid, private_key_encrypted, public_key, algorithm, is_active)
VALUES ($1, $2, $3, $4, $5);

-- name: RotateSigningKey :exec
UPDATE signing_keys
SET is_active      = false,
    deactivated_at = NOW()
WHERE is_active = true;

-- name: RetireExpiredSigningKeys :execrows
UPDATE signing_keys
SET retired_at = NOW()
WHERE is_active = false
  AND retired_at IS NULL
  AND deactivated_at IS NOT NULL
  AND deactivated_at < $1;
