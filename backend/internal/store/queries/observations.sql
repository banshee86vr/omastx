-- name: InsertObservation :exec
INSERT INTO observations (scan_id, artifact_id, installed_version, latest_version,
                          drift_class, drift_score, releases_behind, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListObservationHistory :many
SELECT s.id AS scan_id, s.started_at, o.installed_version, o.latest_version,
       o.drift_class, o.drift_score, o.releases_behind
FROM observations o
JOIN scans s ON s.id = o.scan_id
WHERE o.artifact_id = $1
ORDER BY s.started_at DESC
LIMIT $2;
