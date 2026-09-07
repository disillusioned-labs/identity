-- name: GrantServiceAccess :exec
INSERT INTO organization_service_access (organization_id, user_id, service_name, granted_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (organization_id, user_id, service_name) DO NOTHING;

-- name: RevokeServiceAccess :execrows
DELETE FROM organization_service_access
WHERE organization_id = $1 AND user_id = $2 AND service_name = $3;

-- name: RevokeAllServiceAccess :exec
DELETE FROM organization_service_access
WHERE organization_id = $1 AND user_id = $2;

-- name: ListServiceAccessByOrgUser :many
SELECT id, organization_id, user_id, service_name, granted_by, created_at
FROM organization_service_access
WHERE organization_id = $1 AND user_id = $2
ORDER BY service_name;

-- name: ListServiceAccessByOrg :many
SELECT id, organization_id, user_id, service_name, granted_by, created_at
FROM organization_service_access
WHERE organization_id = $1
ORDER BY user_id, service_name;

-- name: IsServiceAccessAllowed :one
SELECT EXISTS(
    SELECT 1
    FROM organization_service_access
    WHERE organization_id = $1 AND user_id = $2 AND service_name = $3
) AS allowed;
