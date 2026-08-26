-- Created at: 2026-08-26T00:00:00Z

-- @UP
ALTER TABLE applications DROP COLUMN service_url;

-- @DOWN
ALTER TABLE applications ADD COLUMN service_url VARCHAR(512) NOT NULL DEFAULT '';
