-- name: ListGlobalRegistryAuth :many
SELECT id, target, kind, method, username_enc, username_nonce, password_enc, password_nonce, created_at
FROM global_registry_auth
ORDER BY kind, target;

-- name: UpsertGlobalRegistryAuth :one
INSERT INTO global_registry_auth (target, kind, method, username_enc, username_nonce, password_enc, password_nonce)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (target, kind) DO UPDATE SET
    method = EXCLUDED.method,
    username_enc = EXCLUDED.username_enc,
    username_nonce = EXCLUDED.username_nonce,
    password_enc = EXCLUDED.password_enc,
    password_nonce = EXCLUDED.password_nonce
RETURNING id;

-- name: DeleteGlobalRegistryAuth :exec
DELETE FROM global_registry_auth WHERE target = $1 AND kind = $2;
