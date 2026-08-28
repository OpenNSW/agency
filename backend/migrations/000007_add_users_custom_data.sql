-- Created at: 2026-08-28T00:00:00Z

-- @UP
ALTER TABLE users ADD COLUMN custom_data JSONB;

-- This runs inside the migrator's transaction (internal/migrator.Migrator.apply),
-- so it must be a plain CREATE INDEX: CONCURRENTLY cannot run inside a
-- transaction block, and this migrator has no non-transactional migration
-- path today. A plain CREATE INDEX takes a ShareLock that blocks writes to
-- `users` for the build's duration; accepted here because `users` holds only
-- this agency's seeded officers (a handful to low hundreds of rows, written
-- only via the offline `nswac user add` CLI, never a request-path table), so
-- the build is effectively instant. Revisit with CONCURRENTLY (and
-- non-transactional migrator support) if `users` ever becomes a live,
-- high-write table.
-- @postgres
CREATE INDEX IF NOT EXISTS idx_users_custom_data ON users USING GIN (custom_data);

-- @DOWN
ALTER TABLE users DROP COLUMN custom_data;

-- @postgres
DROP INDEX IF EXISTS idx_users_custom_data;
