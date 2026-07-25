-- +goose Up
-- Cluster-scoped registry / Helm repo credentials (references or encrypted basic auth).
-- Secret *contents* from the cluster are never stored; only names or encrypted creds.

CREATE TABLE registry_auth (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id       uuid NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    target           text NOT NULL,
    kind             text NOT NULL CHECK (kind IN ('image', 'helm')),
    method           text NOT NULL CHECK (method IN ('pull_secret', 'basic')),
    secret_namespace text,
    secret_name      text,
    username_enc     bytea,
    username_nonce   bytea,
    password_enc     bytea,
    password_nonce   bytea,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cluster_id, target, kind)
);

CREATE INDEX registry_auth_cluster_id_idx ON registry_auth (cluster_id);

-- +goose Down
DROP TABLE registry_auth;
