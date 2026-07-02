-- Kubeconfig columns are intentionally excluded from list/get queries; the
-- encrypted blob is only readable via GetClusterKubeconfig (scanner use).

-- name: CreateCluster :one
INSERT INTO clusters (name, api_server_url, kubeconfig_enc, kubeconfig_nonce, rbac_report, schedule_cron, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, name, api_server_url, rbac_report, schedule_cron, created_at, last_scan_at, status;

-- name: ListClusters :many
SELECT id, name, api_server_url, rbac_report, schedule_cron, created_at, last_scan_at, status
FROM clusters
ORDER BY name;

-- name: GetCluster :one
SELECT id, name, api_server_url, rbac_report, schedule_cron, created_at, last_scan_at, status
FROM clusters
WHERE id = $1;

-- name: GetClusterKubeconfig :one
SELECT kubeconfig_enc, kubeconfig_nonce
FROM clusters
WHERE id = $1;

-- name: DeleteCluster :execrows
DELETE FROM clusters WHERE id = $1;
