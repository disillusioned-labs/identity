-- name: GetRecoveryUserByEmail :one
SELECT id, email, name
FROM users
WHERE lower(email) = lower(sqlc.arg(email)::text)
  AND deleted_at IS NULL;

-- name: GetUserPasswordByIDForUpdate :one
SELECT password
FROM users
WHERE id = $1
  AND deleted_at IS NULL
FOR UPDATE;

-- name: UpsertPasswordRecoveryToken :one
INSERT INTO password_recovery_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, now() + INTERVAL '15 minutes')
ON CONFLICT (user_id) DO UPDATE
SET token_hash = EXCLUDED.token_hash,
    expires_at = EXCLUDED.expires_at,
    used_at = NULL,
    created_at = now()
WHERE password_recovery_tokens.created_at <= now() - INTERVAL '1 minute'
RETURNING user_id, expires_at;

-- name: GetPasswordRecoveryTokenByHash :one
SELECT t.user_id, u.email, u.name
FROM password_recovery_tokens AS t
JOIN users AS u ON u.id = t.user_id
WHERE t.token_hash = $1
  AND u.deleted_at IS NULL
  AND t.used_at IS NULL
  AND t.expires_at > now()
FOR UPDATE OF t, u;

-- name: UpdateUserPassword :execrows
UPDATE users
SET password = $2
WHERE id = $1
  AND deleted_at IS NULL;

-- name: ConsumePasswordRecoveryToken :execrows
UPDATE password_recovery_tokens
SET used_at = now()
WHERE user_id = $1
  AND used_at IS NULL;
