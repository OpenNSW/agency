-- Created at: 2026-07-30T12:00:00Z

-- @UP
CREATE TABLE IF NOT EXISTS application_files (
    application_id VARCHAR(255) NOT NULL,
    file_key       VARCHAR(255) NOT NULL,
    PRIMARY KEY (application_id, file_key),
    CONSTRAINT fk_application_files_application_id
        FOREIGN KEY (application_id) REFERENCES applications(task_id)
        ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_application_files_file_key ON application_files(file_key);

-- @DOWN
DROP INDEX IF EXISTS idx_application_files_file_key;
DROP TABLE IF EXISTS application_files;
