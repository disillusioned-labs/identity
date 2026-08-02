-- name: CreateUser :one
INSERT INTO users (email, password, name)
VALUES ($1, $2, $3)
RETURNING id, email, name, created_at, updated_at;

-- name: GetUser :one
SELECT id, email, name, created_at, updated_at
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT id, email, password, name, created_at, updated_at
FROM users
WHERE lower(email) = lower($1) AND deleted_at IS NULL;

-- name: ListUsers :many
SELECT id, email, name, created_at, updated_at
FROM users
WHERE deleted_at IS NULL
ORDER BY id
LIMIT $1 OFFSET $2;

-- name: GetUserForUpdate :one
SELECT id, email, name, created_at, updated_at
FROM users
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateUser :one
UPDATE users
SET name = $2, email = $3
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, email, name, created_at, updated_at;

-- name: DeleteUser :execrows
UPDATE users
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL;
