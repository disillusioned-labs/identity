-- name: GetActiveSigningKey :one
SELECT kid, private_key_encrypted, public_key, algorithm
FROM signing_keys
WHERE is_active = true;

-- name: ListActiveSigningKeys :many
SELECT kid, private_key_encrypted, public_key, algorithm
FROM signing_keys
WHERE is_active = true;

-- name: InsertSigningKey :exec
INSERT INTO signing_keys (kid, private_key_encrypted, public_key, algorithm, is_active)
VALUES ($1, $2, $3, $4, $5);

-- name: RotateSigningKey :exec
UPDATE signing_keys SET is_active = false WHERE is_active = true;
