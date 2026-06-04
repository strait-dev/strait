-- PgQue enforces limited-job concurrency from append-only active claims rather
-- than mutating job_active_counts on every claim. Keep the count lookup narrow
-- for constrained queued runs without reintroducing claim-time state updates.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_run_state_pgque_active_claim_counts
    ON job_run_state(job_id, concurrency_key, run_id, ready_generation)
    WHERE status = 'queued'
      AND (job_max_concurrency IS NOT NULL OR job_max_concurrency_per_key IS NOT NULL);
