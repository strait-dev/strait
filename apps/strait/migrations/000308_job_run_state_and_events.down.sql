DROP TRIGGER IF EXISTS trg_job_runs_state_sync_delete ON job_runs;
DROP TRIGGER IF EXISTS trg_job_runs_state_sync_update ON job_runs;
DROP TRIGGER IF EXISTS trg_job_runs_state_sync_insert ON job_runs;
DROP FUNCTION IF EXISTS sync_job_run_state_from_job_runs();
DROP TABLE IF EXISTS job_run_lifecycle_events;
DROP TABLE IF EXISTS job_run_state;

