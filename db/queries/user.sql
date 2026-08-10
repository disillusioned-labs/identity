-- name: CreateUser :one
INSERT INTO users (email, password, name)
VALUES ($1, $2, $3)
    RETURNING id, email, password, name;

-- name: GetUserByEmail :one
SELECT id, email, password, name, last_active_organization_id
FROM users
WHERE lower(email) = lower(sqlc.arg(email)::text)
  AND deleted_at IS NULL;

-- name: SetLastActiveOrganization :execrows
UPDATE users
SET last_active_organization_id = $2
WHERE id = $1
  AND deleted_at IS NULL;
