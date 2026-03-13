ALTER TABLE workflow_versions DROP COLUMN IF EXISTS backwards_compatible;
ALTER TABLE job_versions DROP COLUMN IF EXISTS backwards_compatible;
ALTER TABLE workflows DROP COLUMN IF EXISTS version_policy;
ALTER TABLE jobs DROP COLUMN IF EXISTS version_policy;
