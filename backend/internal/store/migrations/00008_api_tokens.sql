-- +goose Up
-- Machine API tokens (Bearer). Plaintext shown once at creation; only the hash is stored.

CREATE TABLE api_tokens (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name          text NOT NULL,
    token_hash    text NOT NULL UNIQUE,
    token_prefix  text NOT NULL,
    scopes        text[] NOT NULL,
    created_by    text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_used_at  timestamptz,
    expires_at    timestamptz,
    revoked_at    timestamptz
);

CREATE INDEX api_tokens_token_hash_idx ON api_tokens (token_hash)
    WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE api_tokens;
