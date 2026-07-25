-- name: CreateAPIToken :one
INSERT INTO api_tokens (name, token_hash, token_prefix, scopes, created_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, name, token_prefix, scopes, created_by, created_at, last_used_at, expires_at, revoked_at;

-- name: GetAPITokenByHash :one
SELECT id, name, token_hash, token_prefix, scopes, created_by, created_at, last_used_at, expires_at, revoked_at
FROM api_tokens
WHERE token_hash = $1
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > now());

-- name: ListAPITokens :many
SELECT id, name, token_prefix, scopes, created_by, created_at, last_used_at, expires_at, revoked_at
FROM api_tokens
WHERE revoked_at IS NULL
ORDER BY created_at DESC;

-- name: RevokeAPIToken :execrows
UPDATE api_tokens
SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: TouchAPITokenLastUsed :exec
UPDATE api_tokens
SET last_used_at = now()
WHERE id = $1;
