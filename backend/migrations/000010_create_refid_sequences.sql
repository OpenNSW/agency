-- Created at: 2026-09-04T00:00:00Z
--
-- Durable sequence counters for github.com/OpenNSW/core/refid (one row per
-- resolved scope key). Created here rather than via refid's own
-- postgres.Migrate/sqlite.Migrate helpers so the .sql file stays the single
-- source of truth for the schema (docs/migrations.md) and the table gets
-- down/status like every other one. Keep the shape identical to those
-- helpers' DDL — refid's queries run against this table unchanged.
--
-- Dialect-split because the timestamp column genuinely differs: SQLite has no
-- TIMESTAMPTZ and no now(), and rejects the DEFAULT expression outright.

-- @UP
-- @postgres
CREATE TABLE IF NOT EXISTS refid_sequences (
    scope_key  TEXT        NOT NULL PRIMARY KEY,
    counter    BIGINT      NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- @sqlite
CREATE TABLE IF NOT EXISTS refid_sequences (
    scope_key  TEXT    NOT NULL PRIMARY KEY,
    counter    INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- @DOWN
DROP TABLE IF EXISTS refid_sequences;
