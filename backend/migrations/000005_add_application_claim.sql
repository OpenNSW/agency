-- Created at: 2026-08-23T00:00:00Z

-- @UP
ALTER TABLE applications ADD COLUMN claimed_by TEXT REFERENCES users(user_id) ON DELETE SET NULL;
ALTER TABLE applications ADD COLUMN claimed_by_name TEXT;
ALTER TABLE applications ADD COLUMN claimed_by_email TEXT;
ALTER TABLE applications ADD COLUMN claimed_at TIMESTAMP;

-- @DOWN
ALTER TABLE applications DROP COLUMN claimed_by;
ALTER TABLE applications DROP COLUMN claimed_by_name;
ALTER TABLE applications DROP COLUMN claimed_by_email;
ALTER TABLE applications DROP COLUMN claimed_at;
