-- name: CreateUser :one
INSERT INTO users (email, password_hash, role)
VALUES ($1, $2, $3)
ON CONFLICT (email) DO NOTHING
RETURNING id, email, password_hash, role, created_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, role, created_at
FROM users
WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, password_hash, role, created_at
FROM users
WHERE id = $1;

-- name: CountUsers :one
SELECT count(*) FROM users;
