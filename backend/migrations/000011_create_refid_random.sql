-- Created at: 2026-09-13T00:00:00Z
--
-- Issued random ID values for github.com/OpenNSW/core/refid (one row per value
-- per resolved scope key), which is how a random segment detects a collision
-- and retries. Every random-segment format shares this one table, told apart
-- by scope_key. Created here rather than via refid's own Migrate helpers for
-- the same reason as refid_sequences: the .sql file stays the single source of
-- truth for the schema (docs/migrations.md) and the table gets down/status
-- like every other one. Keep the shape identical to those helpers' DDL —
-- refid's queries run against this table unchanged.
--
-- Dialect-split because the timestamp column genuinely differs: SQLite has no
-- TIMESTAMPTZ and no now(), and rejects the DEFAULT expression outright.

-- @UP
-- @postgres
CREATE TABLE IF NOT EXISTS refid_random (
    scope_key  TEXT        NOT NULL,
    value      TEXT        NOT NULL,
    issued_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (scope_key, value)
);

-- @sqlite
CREATE TABLE IF NOT EXISTS refid_random (
    scope_key  TEXT NOT NULL,
    value      TEXT NOT NULL,
    issued_at  TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (scope_key, value)
);

-- @DOWN
DROP TABLE IF EXISTS refid_random;
