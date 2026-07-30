-- Created at: 2026-07-30T12:00:00Z

-- @UP
CREATE TABLE IF NOT EXISTS uploaded_files (
    key            VARCHAR(255) PRIMARY KEY,
    uploaded_by    VARCHAR(255) NOT NULL,
    company_id     VARCHAR(255),
    task_id        VARCHAR(255),
    consignment_id VARCHAR(255),
    created_at     TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_uploaded_files_uploaded_by ON uploaded_files(uploaded_by);
CREATE INDEX IF NOT EXISTS idx_uploaded_files_company_id ON uploaded_files(company_id);

-- @DOWN
DROP INDEX IF EXISTS idx_uploaded_files_company_id;
DROP INDEX IF EXISTS idx_uploaded_files_uploaded_by;
DROP TABLE IF EXISTS uploaded_files;
