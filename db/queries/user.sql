-- name: CreateUser :one
INSERT INTO users (email, password, name)
VALUES ($1, $2, $3)
    RETURNING id, email, password, name;

-- name: GetUserByEmail :one
SELECT id, email, password, name, last_active_organization_id
FROM users
WHERE lower(email) = lower(sqlc.arg(email)::text)
  AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT id, email, name, last_active_organization_id
FROM users
WHERE id = $1
  AND deleted_at IS NULL;

-- name: UserExistsByEmail :one
SELECT EXISTS (
    SELECT 1
    FROM users
    WHERE lower(email) = lower($1)
);

-- name: GetUsersByIDs :many
SELECT id, email, name
FROM users
WHERE id = ANY($1::uuid[])
  AND deleted_at IS NULL;

-- name: GetUsersByOrganization :many
SELECT
    u.id,
    u.email,
    u.name,
    COALESCE(m.role, '') AS role,
    (m.user_id IS NOT NULL)::boolean AS is_active
FROM users u
         LEFT JOIN organization_members m
                   ON m.user_id = u.id
                       AND m.organization_id = $1
                       AND m.deleted_at IS NULL
WHERE u.id = ANY($2::uuid[])
  AND u.deleted_at IS NULL;

-- name: SetLastActiveOrganization :execrows
UPDATE users
SET last_active_organization_id = $2
WHERE id = $1
  AND deleted_at IS NULL;
