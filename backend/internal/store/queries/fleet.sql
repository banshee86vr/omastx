-- FleetLaneRollup returns, for every cluster, the drift-class breakdown of its
-- latest completed scan (SPEC §2.7 GET /api/fleet/summary). Clusters with no
-- completed scan yet still appear with all counts at zero (LEFT JOIN) so the
-- fleet chart can render an empty lane instead of hiding the cluster.
-- name: FleetLaneRollup :many
SELECT c.id AS cluster_id, c.name AS cluster_name, c.status AS cluster_status,
       c.last_scan_at,
       COUNT(o.artifact_id)::int AS total,
       COUNT(o.artifact_id) FILTER (WHERE o.drift_class = 'current')::int AS current,
       COUNT(o.artifact_id) FILTER (WHERE o.drift_class = 'patch')::int AS patch,
       COUNT(o.artifact_id) FILTER (WHERE o.drift_class = 'minor')::int AS minor,
       COUNT(o.artifact_id) FILTER (WHERE o.drift_class = 'major')::int AS major,
       COUNT(o.artifact_id) FILTER (WHERE o.drift_class = 'deprecated')::int AS deprecated,
       COUNT(o.artifact_id) FILTER (WHERE o.drift_class = 'unknown')::int AS unknown
FROM clusters c
LEFT JOIN LATERAL (
    SELECT ls.id
    FROM scans ls
    WHERE ls.cluster_id = c.id AND ls.status = 'done'
    ORDER BY ls.finished_at DESC NULLS LAST, ls.started_at DESC
    LIMIT 1
) latest_scan ON true
LEFT JOIN observations o ON o.scan_id = latest_scan.id
GROUP BY c.id, c.name, c.status, c.last_scan_at
ORDER BY c.name;

-- ListRecentFleetScans returns the most recent scans across every cluster,
-- newest first, for the fleet page's "last scans" rail. Includes failed scans
-- so they can double as "failures needing attention".
-- name: ListRecentFleetScans :many
SELECT s.id, s.cluster_id, c.name AS cluster_name, s.started_at, s.finished_at, s.status, s.error
FROM scans s
JOIN clusters c ON c.id = s.cluster_id
ORDER BY s.started_at DESC
LIMIT $1;
