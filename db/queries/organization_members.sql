-- name: CreateOrganizationMember :one
INSERT INTO organization_members (organization_id, user_id, role)
VALUES ($1, $2, $3)
    RETURNING organization_id, user_id, role;

-- name: ListUserMemberships :many
SELECT m.organization_id, m.role, o.name, o.type
FROM organization_members m
         JOIN organizations o ON o.id = m.organization_id
WHERE m.user_id = $1
  AND m.deleted_at IS NULL
  AND o.deleted_at IS NULL
ORDER BY m.joined_at;

-- name: GetMembership :one
SELECT m.organization_id, m.role, o.name, o.type
FROM organization_members m
         JOIN organizations o ON o.id = m.organization_id
WHERE m.user_id = $1
  AND m.organization_id = $2
  AND m.deleted_at IS NULL
  AND o.deleted_at IS NULL;
