-- name: CreateOrganization :one
INSERT INTO organizations (name,
                           type)
VALUES ($1, $2) RETURNING
    id,
    name,
    type;

-- name: GetOrganization :one
SELECT o.id,
       o.name,
       o.type
FROM organizations o
         JOIN organization_members m
              ON m.organization_id = o.id
WHERE o.id = $1
  AND m.user_id = $2
  AND o.deleted_at IS NULL
  AND m.deleted_at IS NULL;

-- name: GetOrganizationMember :one
SELECT m.user_id,
       u.name,
       u.email,
       m.role,
       m.joined_at
FROM organization_members m
         JOIN users u
              ON u.id = m.user_id
WHERE m.organization_id = $1
  AND m.user_id = $2
  AND m.deleted_at IS NULL
  AND u.deleted_at IS NULL;

-- name: UpdateOrganization :one
UPDATE organizations
SET name = $1
WHERE id = $2
  AND deleted_at IS NULL RETURNING
    id,
    name,
    type;

-- name: SoftDeleteOrganization :execrows
UPDATE organizations
SET deleted_at = now()
WHERE id = $1
  AND deleted_at IS NULL;