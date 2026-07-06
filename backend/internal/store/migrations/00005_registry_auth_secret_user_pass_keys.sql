-- +goose Up
ALTER TABLE registry_auth ADD COLUMN secret_username_key text;
ALTER TABLE registry_auth ADD COLUMN secret_password_key text;

UPDATE registry_auth
SET secret_password_key = secret_key
WHERE secret_key IS NOT NULL AND secret_password_key IS NULL;

ALTER TABLE registry_auth DROP COLUMN secret_key;

-- +goose Down
ALTER TABLE registry_auth ADD COLUMN secret_key text;

UPDATE registry_auth
SET secret_key = secret_password_key
WHERE secret_password_key IS NOT NULL;

ALTER TABLE registry_auth DROP COLUMN secret_username_key;
ALTER TABLE registry_auth DROP COLUMN secret_password_key;
