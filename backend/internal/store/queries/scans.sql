-- name: CreateScan :one
INSERT INTO scans (cluster_id, status)
VALUES ($1, 'running')
RETURNING id, cluster_id, started_at, finished_at, status, error, stats;

-- name: FinishScan :exec
UPDATE scans
SET status = $2, error = $3, stats = $4, finished_at = now()
WHERE id = $1;

-- name: GetScan :one
SELECT id, cluster_id, started_at, finished_at, status, error, stats
FROM scans
WHERE id = $1;

-- name: ListScansByCluster :many
SELECT id, cluster_id, started_at, finished_at, status, error, stats
FROM scans
WHERE cluster_id = $1
ORDER BY started_at DESC
LIMIT $2;

-- name: UpdateClusterScanState :exec
UPDATE clusters
SET last_scan_at = now(), status = $2
WHERE id = $1;
