-- Created at: 2026-08-24T00:00:00Z

-- @UP
ALTER TABLE consignments ADD COLUMN nsw_data JSONB;

-- @DOWN
ALTER TABLE consignments DROP COLUMN nsw_data;
