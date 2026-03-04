ALTER TABLE job_runs DROP COLUMN IF EXISTS workflow_step_run_id;
DROP TABLE IF EXISTS workflow_step_runs;
DROP TABLE IF EXISTS workflow_runs;
