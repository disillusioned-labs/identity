-- name: CreateOrganization :one
INSERT INTO organizations (name, type)
VALUES ($1, $2)
    RETURNING id, name, type;