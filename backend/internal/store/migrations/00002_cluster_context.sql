-- +goose Up

-- The connect flow imports one kubeconfig context per cluster (DECISIONS D8). We
-- store which context so the scanner can rebuild exactly that client. Additive,
-- forward-only. Existing rows default to '' and fall back to the current-context.
ALTER TABLE clusters ADD COLUMN context text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE clusters DROP COLUMN context;
