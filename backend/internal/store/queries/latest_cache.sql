-- name: GetLatestCache :one
SELECT identity, kind, latest_version, candidates, resolved_at, ttl,
       (resolved_at + ttl > now()) AS fresh
FROM latest_cache
WHERE identity = $1 AND kind = $2;

-- name: UpsertLatestCache :exec
INSERT INTO latest_cache (identity, kind, latest_version, candidates, resolved_at, ttl)
VALUES ($1, $2, $3, $4, now(), $5)
ON CONFLICT (identity, kind)
DO UPDATE SET latest_version = EXCLUDED.latest_version,
              candidates = EXCLUDED.candidates,
              resolved_at = now(),
              ttl = EXCLUDED.ttl;
