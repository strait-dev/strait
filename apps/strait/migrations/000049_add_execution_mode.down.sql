DROP INDEX IF EXISTS idx_jobs_execution_mode;
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS chk_sandbox_fields;
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS chk_execution_mode;
ALTER TABLE jobs DROP COLUMN IF EXISTS sandbox_language;
ALTER TABLE jobs DROP COLUMN IF EXISTS sandbox_code;
ALTER TABLE jobs DROP COLUMN IF EXISTS execution_mode;
