-- safety-ok: managed-compute and code-deployments features were removed in
-- migrations 000229 and 000230; the matching job_runs columns are already gone
-- and no live process reads these mirror columns on job_runs_history.
ALTER TABLE job_runs_history
    DROP COLUMN IF EXISTS machine_id,
    DROP COLUMN IF EXISTS deployment_id,
    DROP COLUMN IF EXISTS pinned_image_uri,
    DROP COLUMN IF EXISTS pinned_image_digest;
