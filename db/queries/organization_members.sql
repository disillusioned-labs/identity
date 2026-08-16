-- name: CreateOrganizationMember :one
INSERT INTO organization_members (
    organization_id,
    user_id,
    role
)
VALUES ($1, $2, $3)
    RETURNING
    organization_id,
    user_id,
    role,
    joined_at;

-- name: ListUserOrganizations :many
SELECT
    m.organization_id,
    m.role,
    o.name,
    o.type
FROM organization_members m
         JOIN organizations o
              ON o.id = m.organization_id
WHERE m.user_id = $1
  AND m.deleted_at IS NULL
  AND o.deleted_at IS NULL
ORDER BY m.joined_at;

-- name: GetUserOrganization :one
SELECT
    m.organization_id,
    m.user_id,
    u.name AS user_name,
    u.email,
    m.role,
    m.joined_at,
    o.name AS organization_name,
    o.type AS organization_type
FROM organization_members m
         JOIN organizations o
              ON o.id = m.organization_id
         JOIN users u
              ON u.id = m.user_id
WHERE m.user_id = $1
  AND m.organization_id = $2
  AND m.deleted_at IS NULL
  AND o.deleted_at IS NULL
  AND u.deleted_at IS NULL;

-- name: GetUserOrganizationByEmail :one
SELECT
    m.organization_id,
    m.user_id,
    u.name AS user_name,
    u.email,
    m.role,
    m.joined_at,
    o.name AS organization_name,
    o.type AS organization_type
FROM organization_members m
         JOIN organizations o
              ON o.id = m.organization_id
         JOIN users u
              ON u.id = m.user_id
WHERE m.organization_id = $1
  AND u.email = $2
  AND m.deleted_at IS NULL
  AND o.deleted_at IS NULL
  AND u.deleted_at IS NULL;

-- name: ListOrganizationMembers :many
SELECT
    m.user_id,
    u.name,
    u.email,
    m.role,
    m.joined_at
FROM organization_members m
         JOIN users u
              ON u.id = m.user_id
WHERE m.organization_id = $1
  AND m.deleted_at IS NULL
  AND u.deleted_at IS NULL
ORDER BY m.joined_at;

-- name: UpdateOrganizationMemberRole :execrows
UPDATE organization_members
SET role = $1
WHERE organization_id = $2
  AND user_id = $3
  AND deleted_at IS NULL;

-- name: SoftDeleteOrganizationMember :execrows
UPDATE organization_members
SET deleted_at = now()
WHERE organization_id = $1
  AND user_id = $2
  AND deleted_at IS NULL;

-- name: CountActiveOrganizationOwners :one
SELECT COUNT(*)
FROM organization_members
WHERE organization_id = $1
  AND role = 'owner'
  AND deleted_at IS NULL;

-- name: CountActiveOrganizationMembers :one
SELECT COUNT(*)
FROM organization_members
WHERE organization_id = $1
  AND deleted_at IS NULL;