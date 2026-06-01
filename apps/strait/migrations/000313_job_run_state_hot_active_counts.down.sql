DROP TRIGGER IF EXISTS job_run_state_active_counts_trg ON job_run_state;

DELETE FROM job_active_counts;

INSERT INTO job_active_counts (job_id, concurrency_key, count)
SELECT job_id, COALESCE(concurrency_key, ''), COUNT(*)
FROM job_runs
WHERE status IN ('dequeued', 'executing')
GROUP BY job_id, COALESCE(concurrency_key, '')
ON CONFLICT (job_id, concurrency_key) DO UPDATE SET count = EXCLUDED.count;

DROP TRIGGER IF EXISTS job_runs_active_counts_trg ON job_runs;
CREATE TRIGGER job_runs_active_counts_trg
AFTER INSERT OR UPDATE OR DELETE ON job_runs
FOR EACH ROW EXECUTE FUNCTION job_active_counts_apply();

CREATE INDEX IF NOT EXISTS idx_job_run_state_claim_http
    ON job_run_state(priority DESC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'http';

CREATE INDEX IF NOT EXISTS idx_job_run_state_project_claim
    ON job_run_state(project_id, priority DESC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'http';

CREATE INDEX IF NOT EXISTS idx_job_run_state_worker_claim
    ON job_run_state(project_id, queue_name, environment_id, priority DESC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'worker';
