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

-- name: CountUsersByRole :one
SELECT count(*) FROM users WHERE role = $1;

-- name: ListUsers :many
SELECT id, email, role, created_at
FROM users
ORDER BY email;

-- name: UpdateUser :exec
UPDATE users
SET role = sqlc.arg('role'),
    password_hash = COALESCE(NULLIF(sqlc.arg('password_hash')::text, ''), password_hash)
WHERE id = sqlc.arg('id');

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = $1;

