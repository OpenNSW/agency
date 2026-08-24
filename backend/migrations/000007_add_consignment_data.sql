-- Created at: 2026-08-24T00:00:00Z

-- @UP
ALTER TABLE consignments ADD COLUMN data TEXT;

-- @DOWN
ALTER TABLE consignments DROP COLUMN data;
