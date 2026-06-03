-- Keep claim indexes focused on routing and stable tie-breakers. Including
-- updated_at made every state transition touch an indexed column, preventing
-- HOT updates on the narrow mutable run-state table.
DROP INDEX IF EXISTS idx_job_run_state_claim_http;
DROP INDEX IF EXISTS idx_job_run_state_project_claim;
DROP INDEX IF EXISTS idx_job_run_state_worker_claim;

-- safety-ok: job_run_state claim indexes are rebuilt during the PgQue migration sequence before the queue is serving traffic.
CREATE INDEX IF NOT EXISTS idx_job_run_state_claim_http
    ON job_run_state(priority DESC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'http';

-- safety-ok: job_run_state claim indexes are rebuilt during the PgQue migration sequence before the queue is serving traffic.
CREATE INDEX IF NOT EXISTS idx_job_run_state_project_claim
    ON job_run_state(project_id, priority DESC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'http';

-- safety-ok: job_run_state claim indexes are rebuilt during the PgQue migration sequence before the queue is serving traffic.
CREATE INDEX IF NOT EXISTS idx_job_run_state_worker_claim
    ON job_run_state(project_id, queue_name, environment_id, priority DESC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'worker';
