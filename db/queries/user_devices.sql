-- name: UpsertUserDevice :one
-- Re-registering an active token refreshes its platform/app metadata; a token
-- that was revoked keeps its old row (the partial index only conflicts on
-- active rows) and gets a fresh row instead.
INSERT INTO user_devices (user_id, token, platform, app_version)
VALUES ($1, $2, $3, $4)
ON CONFLICT (token) WHERE revoked_at IS NULL
DO UPDATE SET
    platform    = EXCLUDED.platform,
    app_version = EXCLUDED.app_version,
    updated_at  = now()
RETURNING id, user_id, created_at, updated_at;

-- name: RevokeUserDevice :one
-- Only the token's owner may revoke it; user_id comes from the JWT claims.
-- No row returned = the token was never registered by this user (or is
-- already revoked) and maps to ErrNotFound.
UPDATE user_devices
SET revoked_at = now(),
    updated_at = now()
WHERE user_id = $1
  AND token = $2
  AND revoked_at IS NULL
RETURNING id, platform;

-- name: ListActiveDeviceTokens :many
SELECT token
FROM user_devices
WHERE user_id = $1
  AND revoked_at IS NULL;
