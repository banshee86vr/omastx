-- name: ListRegistryAuth :many
SELECT id, cluster_id, target, kind, method,
       secret_namespace, secret_name, secret_username_key, secret_password_key,
       username_enc, username_nonce, password_enc, password_nonce
FROM registry_auth
WHERE cluster_id = $1
ORDER BY target, kind;

-- name: UpsertRegistryAuth :one
INSERT INTO registry_auth (
    cluster_id, target, kind, method,
    secret_namespace, secret_name, secret_username_key, secret_password_key,
    username_enc, username_nonce, password_enc, password_nonce
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (cluster_id, target, kind)
DO UPDATE SET
    method = EXCLUDED.method,
    secret_namespace = EXCLUDED.secret_namespace,
    secret_name = EXCLUDED.secret_name,
    secret_username_key = EXCLUDED.secret_username_key,
    secret_password_key = EXCLUDED.secret_password_key,
    username_enc = EXCLUDED.username_enc,
    username_nonce = EXCLUDED.username_nonce,
    password_enc = EXCLUDED.password_enc,
    password_nonce = EXCLUDED.password_nonce
RETURNING id;

-- name: DeleteRegistryAuth :exec
DELETE FROM registry_auth
WHERE cluster_id = $1 AND target = $2 AND kind = $3;
