-- name: UpsertArtifact :one
INSERT INTO artifacts (cluster_id, kind, namespace, owner_kind, owner_name, identity, installed_version, source_meta)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (cluster_id, kind, namespace, owner_kind, owner_name, identity)
DO UPDATE SET installed_version = EXCLUDED.installed_version,
              source_meta = EXCLUDED.source_meta,
              last_seen = now()
RETURNING id;

-- ListArtifacts returns each artifact with its most recent observation (the drift
-- from the latest scan that saw it). Filters are optional (NULL = no filter).
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
LEFT JOIN LATERAL (
    SELECT ob.latest_version, ob.drift_class, ob.drift_score, ob.releases_behind, ob.confidence
    FROM observations ob
    JOIN scans s ON s.id = ob.scan_id
    WHERE ob.artifact_id = a.id
    ORDER BY s.started_at DESC
    LIMIT 1
) o ON true
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
LEFT JOIN LATERAL (
    SELECT ob.latest_version, ob.drift_class, ob.drift_score, ob.releases_behind, ob.confidence
    FROM observations ob
    JOIN scans s ON s.id = ob.scan_id
    WHERE ob.artifact_id = a.id
    ORDER BY s.started_at DESC
    LIMIT 1
) o ON true
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
