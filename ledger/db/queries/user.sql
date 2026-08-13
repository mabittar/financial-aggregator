-- name: CreateUser :one
INSERT INTO users (email, password_hash, display_name)
VALUES ($1, $2, $3)
RETURNING id, created_at;

-- name: FindUserByEmail :one
SELECT id, email, password_hash, display_name, created_at
FROM users
WHERE email = $1;