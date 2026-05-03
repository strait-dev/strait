-- Remove code-first deployment infrastructure now that managed-container
-- execution has been replaced by orchestration-only mode.

-- Drop FK columns on job_runs first (reference code_deployments).
ALTER TABLE job_runs DROP COLUMN IF EXISTS deployment_id;
ALTER TABLE job_runs DROP COLUMN IF EXISTS pinned_image_uri;
ALTER TABLE job_runs DROP COLUMN IF EXISTS pinned_image_digest;

-- Drop FK columns on jobs (reference code_deployments).
ALTER TABLE jobs DROP COLUMN IF EXISTS source_type;
ALTER TABLE jobs DROP COLUMN IF EXISTS runtime;
ALTER TABLE jobs DROP COLUMN IF EXISTS active_deployment_id;
ALTER TABLE jobs DROP COLUMN IF EXISTS rollback_source_deployment_id;

-- Drop the code_deployments table after all FK references are gone.
DROP TABLE IF EXISTS code_deployments;
