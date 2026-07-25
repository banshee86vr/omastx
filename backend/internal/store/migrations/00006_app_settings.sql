-- +goose Up
-- Application-wide settings: resolver cache TTLs (singleton row).

CREATE TABLE app_settings (
    id               int PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    oci_ttl          interval NOT NULL DEFAULT '6 hours',
    helmrepo_ttl     interval NOT NULL DEFAULT '6 hours',
    artifacthub_ttl  interval NOT NULL DEFAULT '6 hours',
    updated_at       timestamptz NOT NULL DEFAULT now()
);

INSERT INTO app_settings (id) VALUES (1);

-- Global/default registry credentials (fallback when no per-cluster match).
CREATE TABLE global_registry_auth (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    target             text NOT NULL,
    kind               text NOT NULL CHECK (kind IN ('image', 'helm')),
    method             text NOT NULL CHECK (method IN ('basic')),
    username_enc       bytea,
    username_nonce     bytea,
    password_enc       bytea,
    password_nonce     bytea,
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (target, kind)
);

-- +goose Down
DROP TABLE global_registry_auth;
DROP TABLE app_settings;
