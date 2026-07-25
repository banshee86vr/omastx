-- name: UpsertArtifact :one
INSERT INTO artifacts (cluster_id, kind, namespace, owner_kind, owner_name, identity, installed_version, source_meta)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (cluster_id, kind, namespace, owner_kind, owner_name, identity)
DO UPDATE SET installed_version = EXCLUDED.installed_version,
              source_meta = EXCLUDED.source_meta,
              last_seen = now()
RETURNING id;

-- name: DeleteArtifactsNotInScan :exec
DELETE FROM artifacts a
WHERE a.cluster_id = $1
  AND NOT EXISTS (
    SELECT 1 FROM observations o
    WHERE o.scan_id = $2 AND o.artifact_id = a.id
  );

-- ListArtifacts returns artifacts from each cluster's latest completed scan.
-- Filters are optional (NULL = no filter).
-- Default sort is drift_score desc (SPEC §5.5). One extra row is fetched by the
-- caller (lim = pageSize+1) to detect whether another page exists.
-- name: ListArtifacts :many
SELECT a.id, a.cluster_id, c.name AS cluster_name, a.kind, a.namespace,
       a.owner_kind, a.owner_name, a.identity, a.installed_version, a.last_seen,
       o.latest_version,
       COALESCE(o.drift_class, 'unknown')::text AS drift_class,
       COALESCE(o.drift_score, 0)::double precision AS drift_score,
       o.releases_behind, o.confidence
FROM artifacts a
JOIN clusters c ON c.id = a.cluster_id
JOIN LATERAL (
    SELECT ls.id
    FROM scans ls
    WHERE ls.cluster_id = a.cluster_id AND ls.status = 'done'
    ORDER BY ls.finished_at DESC NULLS LAST, ls.started_at DESC
    LIMIT 1
) latest_scan ON true
JOIN observations o ON o.artifact_id = a.id AND o.scan_id = latest_scan.id
WHERE (sqlc.narg('cluster')::uuid IS NULL OR a.cluster_id = sqlc.narg('cluster'))
  AND (sqlc.narg('kind')::text IS NULL OR a.kind = sqlc.narg('kind'))
  AND (sqlc.narg('namespace')::text IS NULL OR a.namespace = sqlc.narg('namespace'))
  AND (sqlc.narg('class')::text IS NULL OR o.drift_class = sqlc.narg('class'))
  AND (sqlc.narg('resolve_status')::text IS NULL
       OR COALESCE(a.source_meta->>'resolve_status', '') = sqlc.narg('resolve_status'))
  AND (sqlc.narg('q')::text IS NULL OR a.identity ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY COALESCE(o.drift_score, -1) DESC, a.identity ASC, a.id ASC
OFFSET sqlc.arg('off')
LIMIT sqlc.arg('lim');

-- ListArtifactsForExport returns all matching artifacts (no pagination) for CSV/PDF export.
-- name: ListArtifactsForExport :many
SELECT a.id, a.cluster_id, c.name AS cluster_name, a.kind, a.namespace,
       a.owner_kind, a.owner_name, a.identity, a.installed_version, a.last_seen,
       o.latest_version,
       COALESCE(o.drift_class, 'unknown')::text AS drift_class,
       COALESCE(o.drift_score, 0)::double precision AS drift_score,
       o.releases_behind, o.confidence
FROM artifacts a
JOIN clusters c ON c.id = a.cluster_id
JOIN LATERAL (
    SELECT ls.id
    FROM scans ls
    WHERE ls.cluster_id = a.cluster_id AND ls.status = 'done'
    ORDER BY ls.finished_at DESC NULLS LAST, ls.started_at DESC
    LIMIT 1
) latest_scan ON true
JOIN observations o ON o.artifact_id = a.id AND o.scan_id = latest_scan.id
WHERE (sqlc.narg('cluster')::uuid IS NULL OR a.cluster_id = sqlc.narg('cluster'))
  AND (sqlc.narg('kind')::text IS NULL OR a.kind = sqlc.narg('kind'))
  AND (sqlc.narg('namespace')::text IS NULL OR a.namespace = sqlc.narg('namespace'))
  AND (sqlc.narg('class')::text IS NULL OR o.drift_class = sqlc.narg('class'))
  AND (sqlc.narg('resolve_status')::text IS NULL
       OR COALESCE(a.source_meta->>'resolve_status', '') = sqlc.narg('resolve_status'))
  AND (sqlc.narg('q')::text IS NULL OR a.identity ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY COALESCE(o.drift_score, -1) DESC, a.identity ASC, a.id ASC;

-- name: GetArtifact :one
SELECT a.id, a.cluster_id, c.name AS cluster_name, a.kind, a.namespace,
       a.owner_kind, a.owner_name, a.identity, a.installed_version,
       a.source_meta, a.first_seen, a.last_seen,
       o.latest_version,
       COALESCE(o.drift_class, 'unknown')::text AS drift_class,
       COALESCE(o.drift_score, 0)::double precision AS drift_score,
       o.releases_behind, o.confidence
FROM artifacts a
JOIN clusters c ON c.id = a.cluster_id
JOIN LATERAL (
    SELECT ls.id
    FROM scans ls
    WHERE ls.cluster_id = a.cluster_id AND ls.status = 'done'
    ORDER BY ls.finished_at DESC NULLS LAST, ls.started_at DESC
    LIMIT 1
) latest_scan ON true
JOIN observations o ON o.artifact_id = a.id AND o.scan_id = latest_scan.id
WHERE a.id = $1;

-- name: CountObservationKindsForLatestScan :many
SELECT a.kind, COUNT(*)::int AS count
FROM observations o
JOIN artifacts a ON a.id = o.artifact_id
WHERE o.scan_id = (
    SELECT s.id
    FROM scans s
    WHERE s.cluster_id = $1 AND s.status = 'done'
    ORDER BY s.finished_at DESC NULLS LAST, s.started_at DESC
    LIMIT 1
)
GROUP BY a.kind;

-- name: CountAuthRequiredForLatestScan :one
SELECT COUNT(*)::int AS count
FROM observations o
JOIN artifacts a ON a.id = o.artifact_id
WHERE o.scan_id = (
    SELECT s.id
    FROM scans s
    WHERE s.cluster_id = $1 AND s.status = 'done'
    ORDER BY s.finished_at DESC NULLS LAST, s.started_at DESC
    LIMIT 1
)
AND COALESCE(a.source_meta->>'resolve_status', '') = 'auth_required';

-- ListDriftRegistryTargets returns distinct registry/chart-repo targets for artifacts
-- in the latest completed scan whose observation is unknown drift (for credential picker UI).
-- name: ListDriftRegistryTargets :many
SELECT DISTINCT a.kind,
       CASE
           WHEN a.kind = 'image' THEN NULLIF(TRIM(a.source_meta->>'registry'), '')
           WHEN a.kind = 'helm' THEN NULLIF(TRIM(a.source_meta->>'chart_repo'), '')
           ELSE NULL
       END AS target
FROM artifacts a
JOIN LATERAL (
    SELECT ls.id
    FROM scans ls
    WHERE ls.cluster_id = a.cluster_id AND ls.status = 'done'
    ORDER BY ls.finished_at DESC NULLS LAST, ls.started_at DESC
    LIMIT 1
) latest_scan ON true
JOIN observations o ON o.artifact_id = a.id AND o.scan_id = latest_scan.id
WHERE a.cluster_id = $1
  AND o.drift_class = 'unknown'
  AND (
      (a.kind = 'image' AND NULLIF(TRIM(a.source_meta->>'registry'), '') IS NOT NULL)
      OR (a.kind = 'helm' AND NULLIF(TRIM(a.source_meta->>'chart_repo'), '') IS NOT NULL)
  )
ORDER BY a.kind, target;
