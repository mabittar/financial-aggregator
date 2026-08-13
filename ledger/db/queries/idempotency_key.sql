-- name: CreateIdempotencyKey :one
INSERT INTO idempotency_keys (key, user_id, payload_hash, response_metadata)
VALUES ($1, $2, $3, $4)
ON CONFLICT (key) DO UPDATE SET
    payload_hash = EXCLUDED.payload_hash,
    response_metadata = EXCLUDED.response_metadata,
    updated_at = now()
RETURNING key, user_id, payload_hash, response_metadata, updated_at;