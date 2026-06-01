DROP INDEX IF EXISTS idx_job_run_state_claim_http;
DROP INDEX IF EXISTS idx_job_run_state_project_claim;
DROP INDEX IF EXISTS idx_job_run_state_worker_claim;

CREATE INDEX IF NOT EXISTS idx_job_run_state_claim_http
    ON job_run_state(priority DESC, updated_at ASC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'http';

CREATE INDEX IF NOT EXISTS idx_job_run_state_project_claim
    ON job_run_state(project_id, priority DESC, updated_at ASC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'http';

CREATE INDEX IF NOT EXISTS idx_job_run_state_worker_claim
    ON job_run_state(project_id, queue_name, environment_id, priority DESC, updated_at ASC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'worker';
