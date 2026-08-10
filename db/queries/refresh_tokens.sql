-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, user_agent, ip_address, expires_at)
VALUES ($1, $2, $3, $4, $5)
    RETURNING id, expires_at;

-- name: GetRefreshTokenByHash :one
SELECT id, user_id, expires_at, revoked_at
FROM refresh_tokens
WHERE token_hash = $1;

-- name: RevokeRefreshToken :execrows
UPDATE refresh_tokens
SET revoked_at   = now(),
    last_used_at = now()
WHERE id = $1
  AND revoked_at IS NULL;

-- name: RevokeAllUserRefreshTokens :execrows
UPDATE refresh_tokens
SET revoked_at = now()
WHERE user_id = $1
  AND revoked_at IS NULL;
