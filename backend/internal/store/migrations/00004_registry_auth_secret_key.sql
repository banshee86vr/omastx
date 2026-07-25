-- +goose Up
ALTER TABLE registry_auth ADD COLUMN secret_key text;

-- +goose Down
ALTER TABLE registry_auth DROP COLUMN secret_key;
