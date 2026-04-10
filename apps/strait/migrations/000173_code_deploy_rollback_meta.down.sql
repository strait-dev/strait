ALTER TABLE jobs DROP COLUMN IF EXISTS rollback_source_deployment_id;
ALTER TABLE job_runs DROP COLUMN IF EXISTS is_rollback;
