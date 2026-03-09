ALTER TABLE workflow_versions DROP COLUMN IF EXISTS updated_by;
ALTER TABLE workflow_versions DROP COLUMN IF EXISTS created_by;
ALTER TABLE job_versions DROP COLUMN IF EXISTS updated_by;
ALTER TABLE job_versions DROP COLUMN IF EXISTS created_by;
