-- name: CreateSession :exec
INSERT INTO sessions (token_hash, github_login, github_name, github_avatar_url, csrf_token, expires_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetSession :one
SELECT token_hash, github_login, github_name, github_avatar_url, csrf_token, expires_at
FROM sessions
WHERE token_hash = $1 AND expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();
