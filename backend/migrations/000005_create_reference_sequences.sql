-- Created at: 2026-07-28T00:00:00Z

-- @UP
CREATE TABLE IF NOT EXISTS reference_sequences (
    agency_code   VARCHAR(100) NOT NULL,
    prefix        VARCHAR(50)  NOT NULL,
    current_value BIGINT       NOT NULL DEFAULT 0,
    updated_at    TIMESTAMP    NOT NULL,
    PRIMARY KEY (agency_code, prefix)
);
ALTER TABLE applications ADD COLUMN reference_number VARCHAR(100);

-- @DOWN
ALTER TABLE applications DROP COLUMN reference_number;
DROP TABLE IF EXISTS reference_sequences;
