-- +goose Up

-- SPEC §2.5 data model. All tables created up front in M1; only users/sessions are
-- exercised until M2+.

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    role          text NOT NULL DEFAULT 'admin',
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Additive to SPEC (see docs/plan/DECISIONS.md D3): server-side state for cookie
-- session auth required by §2.6. token_hash is sha256(session token), hex-encoded.
CREATE TABLE sessions (
    token_hash text PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE clusters (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name             text NOT NULL UNIQUE,
    api_server_url   text NOT NULL,
    kubeconfig_enc   bytea NOT NULL,
    kubeconfig_nonce bytea NOT NULL,
    rbac_report      jsonb,
    schedule_cron    text NOT NULL DEFAULT '0 */6 * * *',
    created_at       timestamptz NOT NULL DEFAULT now(),
    last_scan_at     timestamptz,
    status           text NOT NULL DEFAULT 'unknown'
);

CREATE TABLE scans (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id uuid NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    status     text NOT NULL DEFAULT 'running',
    error      text,
    stats      jsonb
);

CREATE INDEX scans_cluster_id_idx ON scans (cluster_id, started_at DESC);

CREATE TABLE artifacts (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id        uuid NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    kind              text NOT NULL,
    namespace         text NOT NULL,
    owner_kind        text NOT NULL,
    owner_name        text NOT NULL,
    identity          text NOT NULL,
    installed_version text NOT NULL,
    source_meta       jsonb,
    first_seen        timestamptz NOT NULL DEFAULT now(),
    last_seen         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX artifacts_cluster_id_idx ON artifacts (cluster_id);
CREATE INDEX artifacts_identity_idx ON artifacts (identity);
CREATE UNIQUE INDEX artifacts_unique_idx
    ON artifacts (cluster_id, kind, namespace, owner_kind, owner_name, identity);

CREATE TABLE observations (
    scan_id           uuid NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    artifact_id       uuid NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    installed_version text NOT NULL,
    latest_version    text,
    drift_class       text NOT NULL DEFAULT 'unknown',
    drift_score       double precision NOT NULL DEFAULT 0,
    releases_behind   integer,
    confidence        real,
    PRIMARY KEY (scan_id, artifact_id)
);

CREATE INDEX observations_artifact_id_idx ON observations (artifact_id);

CREATE TABLE latest_cache (
    identity       text NOT NULL,
    kind           text NOT NULL,
    latest_version text,
    candidates     jsonb,
    resolved_at    timestamptz NOT NULL DEFAULT now(),
    ttl            interval NOT NULL DEFAULT '6 hours',
    PRIMARY KEY (identity, kind)
);

-- +goose Down
DROP TABLE latest_cache;
DROP TABLE observations;
DROP TABLE artifacts;
DROP TABLE scans;
DROP TABLE clusters;
DROP TABLE sessions;
DROP TABLE users;
