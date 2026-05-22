DROP INDEX IF EXISTS idx_job_run_queue_worker_routing;

CREATE OR REPLACE FUNCTION trg_job_runs_sync_claim_queue()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public, pg_catalog
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        DELETE FROM job_run_queue WHERE run_id = OLD.id;
        RETURN OLD;
    END IF;

    IF TG_OP = 'INSERT' THEN
        IF NEW.status IN ('queued', 'delayed') THEN
            INSERT INTO job_run_queue (
                run_id, job_id, project_id, priority, created_at,
                scheduled_at, next_retry_at, concurrency_key,
                job_max_concurrency, job_max_concurrency_per_key,
                job_enabled, job_paused, queue_name
            )
            SELECT
                NEW.id, NEW.job_id, NEW.project_id, NEW.priority, NEW.created_at,
                NEW.scheduled_at, NEW.next_retry_at, NEW.concurrency_key,
                j.max_concurrency, j.max_concurrency_per_key,
                j.enabled, j.paused, NEW.queue_name
            FROM jobs j
            WHERE j.id = NEW.job_id
            ON CONFLICT (run_id) DO NOTHING;
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.status IN ('queued', 'delayed') THEN
        INSERT INTO job_run_queue (
            run_id, job_id, project_id, priority, created_at,
            scheduled_at, next_retry_at, concurrency_key,
            job_max_concurrency, job_max_concurrency_per_key,
            job_enabled, job_paused, queue_name
        )
        SELECT
            NEW.id, NEW.job_id, NEW.project_id, NEW.priority, NEW.created_at,
            NEW.scheduled_at, NEW.next_retry_at, NEW.concurrency_key,
            j.max_concurrency, j.max_concurrency_per_key,
            j.enabled, j.paused, NEW.queue_name
        FROM jobs j
        WHERE j.id = NEW.job_id
        ON CONFLICT (run_id) DO UPDATE SET
            priority = EXCLUDED.priority,
            scheduled_at = EXCLUDED.scheduled_at,
            next_retry_at = EXCLUDED.next_retry_at,
            concurrency_key = EXCLUDED.concurrency_key,
            job_max_concurrency = EXCLUDED.job_max_concurrency,
            job_max_concurrency_per_key = EXCLUDED.job_max_concurrency_per_key,
            job_enabled = EXCLUDED.job_enabled,
            job_paused = EXCLUDED.job_paused,
            queue_name = EXCLUDED.queue_name;
    ELSE
        DELETE FROM job_run_queue WHERE run_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

ALTER TABLE job_run_queue
    DROP COLUMN IF EXISTS execution_mode;
