-- safety-ok: job_run_state is a newly introduced PgQue side table in this migration sequence, populated before serving PgQue traffic.
ALTER TABLE job_run_state
    ADD COLUMN IF NOT EXISTS environment_id TEXT NOT NULL DEFAULT '';

UPDATE job_run_state s
SET environment_id = COALESCE(j.environment_id, '')
FROM job_runs jr
JOIN jobs j ON j.id = jr.job_id
WHERE s.run_id = jr.id
  AND s.environment_id = '';

DROP INDEX IF EXISTS idx_job_run_state_worker_claim;
-- safety-ok: job_run_state is a new PgQue side table rebuilt during startup migration, before workers claim from it.
CREATE INDEX IF NOT EXISTS idx_job_run_state_worker_claim
    ON job_run_state(project_id, queue_name, environment_id, priority DESC, updated_at ASC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'worker';

CREATE OR REPLACE FUNCTION sync_job_run_state_from_job_runs()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    job_environment_id TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        DELETE FROM job_run_state WHERE run_id = OLD.id;
        RETURN OLD;
    END IF;

    SELECT COALESCE(j.environment_id, '')
    INTO job_environment_id
    FROM jobs j
    WHERE j.id = NEW.job_id;

    INSERT INTO job_run_state (
        run_id,
        project_id,
        job_id,
        status,
        attempt,
        priority,
        scheduled_at,
        started_at,
        finished_at,
        heartbeat_at,
        next_retry_at,
        expires_at,
        concurrency_key,
        execution_mode,
        queue_name,
        environment_id,
        job_enabled,
        job_paused,
        job_max_concurrency,
        job_max_concurrency_per_key,
        updated_at
    )
    VALUES (
        NEW.id,
        NEW.project_id,
        NEW.job_id,
        NEW.status,
        NEW.attempt,
        NEW.priority,
        NEW.scheduled_at,
        NEW.started_at,
        NEW.finished_at,
        NEW.heartbeat_at,
        NEW.next_retry_at,
        NEW.expires_at,
        COALESCE(NEW.concurrency_key, ''),
        COALESCE(NULLIF(NEW.execution_mode, ''), 'http'),
        COALESCE(NULLIF(NEW.queue_name, ''), 'default'),
        COALESCE(job_environment_id, ''),
        COALESCE(NEW.job_enabled, TRUE),
        COALESCE(NEW.job_paused, FALSE),
        NEW.job_max_concurrency,
        NEW.job_max_concurrency_per_key,
        NOW()
    )
    ON CONFLICT (run_id) DO UPDATE
    SET project_id = EXCLUDED.project_id,
        job_id = EXCLUDED.job_id,
        status = EXCLUDED.status,
        attempt = EXCLUDED.attempt,
        priority = EXCLUDED.priority,
        scheduled_at = EXCLUDED.scheduled_at,
        started_at = EXCLUDED.started_at,
        finished_at = EXCLUDED.finished_at,
        heartbeat_at = EXCLUDED.heartbeat_at,
        next_retry_at = EXCLUDED.next_retry_at,
        expires_at = EXCLUDED.expires_at,
        concurrency_key = EXCLUDED.concurrency_key,
        execution_mode = EXCLUDED.execution_mode,
        queue_name = EXCLUDED.queue_name,
        environment_id = EXCLUDED.environment_id,
        job_enabled = EXCLUDED.job_enabled,
        job_paused = EXCLUDED.job_paused,
        job_max_concurrency = EXCLUDED.job_max_concurrency,
        job_max_concurrency_per_key = EXCLUDED.job_max_concurrency_per_key,
        updated_at = NOW();

    RETURN NEW;
END;
$$;
