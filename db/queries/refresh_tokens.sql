-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, user_agent, ip_address, expires_at)
VALUES ($1, $2, $3, $4, $5)
    RETURNING id, expires_at;
