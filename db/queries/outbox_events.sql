-- name: CreateOutboxEvent :one
INSERT INTO outbox_events (
    aggregate_type,
    aggregate_id,
    event_type,
    event_version,
    payload
)
VALUES (
           $1,
           $2,
           $3,
           $4,
           $5
       )
    RETURNING
    id,
    aggregate_type,
    aggregate_id,
    event_type,
    event_version,
    payload,
    created_at,
    published_at,
    attempt_count,
    next_attempt_at,
    locked_at,
    locked_by,
    last_error;


-- name: GetPendingOutboxEvents :many
SELECT
    id,
    aggregate_type,
    aggregate_id,
    event_type,
    event_version,
    payload,
    created_at,
    published_at,
    attempt_count,
    next_attempt_at,
    locked_at,
    locked_by,
    last_error
FROM outbox_events
WHERE published_at IS NULL
  AND (
    next_attempt_at IS NULL
        OR next_attempt_at <= NOW()
    )
  AND (
    locked_at IS NULL
        OR locked_at < NOW() - INTERVAL '5 minutes'
    )
ORDER BY created_at ASC, id ASC
    LIMIT $1
FOR UPDATE SKIP LOCKED;


-- name: LockOutboxEvent :exec
UPDATE outbox_events
SET
    locked_at = NOW(),
    locked_by = $2
WHERE id = $1
  AND published_at IS NULL;


-- name: MarkOutboxEventPublished :exec
UPDATE outbox_events
SET
    published_at = NOW(),
    locked_at = NULL,
    locked_by = NULL,
    last_error = NULL
WHERE id = $1
  AND published_at IS NULL;


-- name: MarkOutboxEventFailed :exec
UPDATE outbox_events
SET
    attempt_count = attempt_count + 1,
    next_attempt_at = $2,
    last_error = $3,
    locked_at = NULL,
    locked_by = NULL
WHERE id = $1
  AND published_at IS NULL;


-- name: ReleaseOutboxEventLock :exec
UPDATE outbox_events
SET
    locked_at = NULL,
    locked_by = NULL
WHERE id = $1
  AND published_at IS NULL;


-- name: DeletePublishedOutboxEvents :execrows
DELETE FROM outbox_events
WHERE published_at IS NOT NULL
  AND published_at < NOW() - INTERVAL '7 days';