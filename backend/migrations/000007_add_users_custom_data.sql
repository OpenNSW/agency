-- Created at: 2026-08-28T00:00:00Z

-- @UP
ALTER TABLE users ADD COLUMN custom_data JSONB;

-- @postgres
CREATE INDEX IF NOT EXISTS idx_users_custom_data ON users USING GIN (custom_data);

-- @DOWN
ALTER TABLE users DROP COLUMN custom_data;

-- @postgres
DROP INDEX IF EXISTS idx_users_custom_data;