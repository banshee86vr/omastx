-- +goose Up
-- Replace local email/password users with GitHub OAuth identity on sessions (D19).

ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_user_id_fkey;
ALTER TABLE sessions DROP COLUMN user_id;
ALTER TABLE sessions ADD COLUMN github_login text NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN github_name text NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN github_avatar_url text NOT NULL DEFAULT '';

DROP TABLE users;

-- +goose Down
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    role          text NOT NULL DEFAULT 'admin',
    created_at    timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE sessions DROP COLUMN github_avatar_url;
ALTER TABLE sessions DROP COLUMN github_name;
ALTER TABLE sessions DROP COLUMN github_login;
ALTER TABLE sessions ADD COLUMN user_id uuid REFERENCES users(id) ON DELETE CASCADE;
