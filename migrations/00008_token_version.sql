-- +goose Up
-- Tokens carry the user's token_version. Bumping it (password reset, suspension)
-- invalidates every token issued before.
ALTER TABLE users ADD COLUMN token_version INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE users DROP COLUMN token_version;
