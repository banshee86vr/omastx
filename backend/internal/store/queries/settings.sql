-- name: GetAppSettings :one
SELECT oci_ttl, helmrepo_ttl, artifacthub_ttl, updated_at
FROM app_settings
WHERE id = 1;

-- name: UpdateAppSettings :exec
UPDATE app_settings
SET oci_ttl = $1, helmrepo_ttl = $2, artifacthub_ttl = $3, updated_at = now()
WHERE id = 1;
