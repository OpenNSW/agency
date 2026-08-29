-- Created at: 2026-08-28T00:00:00Z
--
-- No GIN index here, unlike users.custom_data (migration 000007): consignments
-- is a live, high(er)-write, request-path table (touched on every application
-- inject/review/feedback/claim), not a small CLI-seed-only one. The column add
-- itself is a fast, safe metadata-only change on Postgres, but a plain
-- CREATE INDEX (required inside this migrator's transaction; CONCURRENTLY
-- cannot run in a transaction) would take a ShareLock for the build's
-- duration, which isn't a risk worth accepting blindly on this table the way
-- it was for `users`. Add the index as its own deliberate follow-up once
-- there's visibility into actual row counts.

-- @UP
ALTER TABLE consignments ADD COLUMN custom_data JSONB;

-- @DOWN
ALTER TABLE consignments DROP COLUMN custom_data;
