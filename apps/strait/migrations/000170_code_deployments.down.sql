ALTER TABLE job_runs DROP COLUMN IF EXISTS pinned_image_digest;
ALTER TABLE job_runs DROP COLUMN IF EXISTS pinned_image_uri;
ALTER TABLE job_runs DROP COLUMN IF EXISTS deployment_id;

ALTER TABLE jobs DROP COLUMN IF EXISTS active_deployment_id;
ALTER TABLE jobs DROP COLUMN IF EXISTS runtime;
ALTER TABLE jobs DROP COLUMN IF EXISTS source_type;

DROP TABLE IF EXISTS code_deployments;
