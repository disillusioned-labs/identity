-- name: ListMyPendingOrganizationInvitations :many
SELECT oi.id,
       oi.organization_id,
       o.name AS organization_name,
       oi.role,
       oi.status,
       oi.expires_at,
       oi.created_at
FROM organization_invitations oi
         JOIN organizations o
              ON o.id = oi.organization_id
WHERE lower(oi.email) = lower($1)
  AND oi.status = 'pending'
  AND oi.expires_at > NOW()
ORDER BY oi.created_at DESC;


-- name: CreateInvitation :one
INSERT INTO organization_invitations (organization_id,
                                      email,
                                      role,
                                      token_hash,
                                      invited_by,
                                      expires_at)
VALUES ($1,
        $2,
        $3,
        $4,
        $5,
        $6) RETURNING
    id,
    organization_id,
    email,
    role,
    token_hash,
    invited_by,
    status,
    expires_at,
    accepted_at,
    accepted_by,
    created_at;


-- name: GetInvitationByTokenHash :one
SELECT oi.id,
       oi.organization_id,
       oi.email,
       oi.role,
       oi.token_hash,
       oi.invited_by,
       oi.status,
       oi.expires_at,
       oi.accepted_at,
       oi.accepted_by,
       oi.created_at,
       o.name AS organization_name,
       u.name AS invited_by_name
FROM organization_invitations oi
         JOIN organizations o
              ON o.id = oi.organization_id
         JOIN users u
              ON u.id = oi.invited_by
WHERE oi.token_hash = $1 LIMIT 1;


-- name: GetPendingOrganizationInvitation :one
SELECT id,
       organization_id,
       email,
       role,
       token_hash,
       invited_by,
       status,
       expires_at,
       accepted_at,
       accepted_by,
       created_at
FROM organization_invitations
WHERE organization_id = $1
  AND lower(email) = lower(sqlc.arg(email))
  AND status = 'pending' LIMIT 1;


-- name: GetOrganizationInvitation :one
SELECT id,
       organization_id,
       email,
       role,
       token_hash,
       invited_by,
       status,
       expires_at,
       accepted_at,
       accepted_by,
       created_at
FROM organization_invitations
WHERE id = $1 LIMIT 1;


-- name: ListInvitations :many
SELECT oi.id,
       oi.organization_id,
       oi.email,
       oi.role,
       oi.token_hash,
       oi.invited_by,
       oi.status,
       oi.expires_at,
       oi.accepted_at,
       oi.accepted_by,
       oi.created_at,
       u.name AS invited_by_name
FROM organization_invitations oi
         JOIN users u
              ON u.id = oi.invited_by
WHERE oi.organization_id = $1
ORDER BY oi.created_at DESC;


-- name: RevokeInvitation :execrows
UPDATE organization_invitations
SET status = 'revoked'
WHERE id = $1
  AND organization_id = $2
  AND status = 'pending';


-- name: AcceptInvitation :execrows
UPDATE organization_invitations
SET status      = 'accepted',
    accepted_at = NOW(),
    accepted_by = $2
WHERE id = $1
  AND status = 'pending'
  AND expires_at > NOW();


-- name: ExpireInvitation :execrows
UPDATE organization_invitations
SET status = 'expired'
WHERE id = $1
  AND status = 'pending'
  AND expires_at <= NOW();