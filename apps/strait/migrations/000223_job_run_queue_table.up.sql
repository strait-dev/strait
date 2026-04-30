-- Thin claim table for the dequeue hot path. ~80 bytes per row vs ~2KB+
-- in job_runs. The B-tree scan operates entirely on this table so it
-- never touches the fat job_runs heap during candidate selection.
--
-- Dequeue DELETEs from this table (not UPDATE), so each claim creates
-- one small dead tuple that aggressive vacuum reclaims in milliseconds.

CREATE TABLE IF NOT EXISTS job_run_queue (
    run_id                      TEXT          NOT NULL,
    job_id                      TEXT          NOT NULL,
    project_id                  TEXT          NOT NULL,
    priority                    INT           NOT NULL DEFAULT 0,
    created_at                  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    scheduled_at                TIMESTAMPTZ,
    next_retry_at               TIMESTAMPTZ,
    concurrency_key             TEXT,
    job_max_concurrency         INT,
    job_max_concurrency_per_key INT,
    job_enabled                 BOOLEAN       NOT NULL DEFAULT true,
    job_paused                  BOOLEAN       NOT NULL DEFAULT false,
    CONSTRAINT job_run_queue_pkey PRIMARY KEY (run_id)
);

-- The dequeue scan index. Matches the ORDER BY in DequeueNClaim.
CREATE INDEX IF NOT EXISTS idx_job_run_queue_dequeue
    ON job_run_queue (priority DESC, created_at ASC);

-- Aggressive vacuum for high INSERT+DELETE churn.
ALTER TABLE job_run_queue SET (
    autovacuum_vacuum_threshold       = 50,
    autovacuum_vacuum_scale_factor    = 0.005,
    autovacuum_vacuum_cost_delay      = 0,
    autovacuum_vacuum_cost_limit      = 2000,
    autovacuum_analyze_threshold      = 50,
    autovacuum_analyze_scale_factor   = 0.005,
    fillfactor                        = 90
);

-- Backfill from existing queued/delayed runs.
INSERT INTO job_run_queue (
    run_id, job_id, project_id, priority, created_at,
    scheduled_at, next_retry_at, concurrency_key,
    job_max_concurrency, job_max_concurrency_per_key,
    job_enabled, job_paused
)
SELECT
    jr.id, jr.job_id, jr.project_id, jr.priority, jr.created_at,
    jr.scheduled_at, jr.next_retry_at, jr.concurrency_key,
    jr.job_max_concurrency, jr.job_max_concurrency_per_key,
    COALESCE(jr.job_enabled, true), COALESCE(jr.job_paused, false)
FROM job_runs jr
WHERE jr.status IN ('queued', 'delayed')
ON CONFLICT (run_id) DO NOTHING;

-- Fan-out trigger: when a job's enabled/paused/concurrency settings change,
-- propagate to pending claim rows so the dequeue scan sees current values.
CREATE OR REPLACE FUNCTION trg_jobs_fanout_to_queue()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public, pg_catalog
AS $$
BEGIN
    IF NEW.enabled IS DISTINCT FROM OLD.enabled
       OR NEW.paused IS DISTINCT FROM OLD.paused
       OR NEW.max_concurrency IS DISTINCT FROM OLD.max_concurrency
       OR NEW.max_concurrency_per_key IS DISTINCT FROM OLD.max_concurrency_per_key
    THEN
        UPDATE job_run_queue
        SET job_enabled = NEW.enabled,
            job_paused = NEW.paused,
            job_max_concurrency = NEW.max_concurrency,
            job_max_concurrency_per_key = NEW.max_concurrency_per_key
        WHERE job_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_jobs_fanout_queue
    AFTER UPDATE ON jobs
    FOR EACH ROW
    EXECUTE FUNCTION trg_jobs_fanout_to_queue();
