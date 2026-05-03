-- Add queue_name to jobs, job_runs, and job_run_queue to support named worker queues.

ALTER TABLE jobs ADD COLUMN IF NOT EXISTS queue_name TEXT NOT NULL DEFAULT 'default';
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS queue_name TEXT NOT NULL DEFAULT 'default';
ALTER TABLE job_run_queue ADD COLUMN IF NOT EXISTS queue_name TEXT NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_job_run_queue_queue_name ON job_run_queue (queue_name);

-- Update the fan-out trigger to also propagate queue_name changes.
CREATE OR REPLACE FUNCTION trg_jobs_fanout_to_queue()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public, pg_catalog
AS $$
DECLARE
    affected int;
BEGIN
    IF NEW.enabled IS DISTINCT FROM OLD.enabled
       OR NEW.paused IS DISTINCT FROM OLD.paused
       OR NEW.max_concurrency IS DISTINCT FROM OLD.max_concurrency
       OR NEW.max_concurrency_per_key IS DISTINCT FROM OLD.max_concurrency_per_key
       OR NEW.queue_name IS DISTINCT FROM OLD.queue_name
    THEN
        LOOP
            UPDATE job_run_queue
            SET job_enabled = NEW.enabled,
                job_paused = NEW.paused,
                job_max_concurrency = NEW.max_concurrency,
                job_max_concurrency_per_key = NEW.max_concurrency_per_key,
                queue_name = NEW.queue_name
            WHERE run_id IN (
                SELECT run_id FROM job_run_queue
                WHERE job_id = NEW.id
                  AND (job_enabled IS DISTINCT FROM NEW.enabled
                       OR job_paused IS DISTINCT FROM NEW.paused
                       OR job_max_concurrency IS DISTINCT FROM NEW.max_concurrency
                       OR job_max_concurrency_per_key IS DISTINCT FROM NEW.max_concurrency_per_key
                       OR queue_name IS DISTINCT FROM NEW.queue_name)
                ORDER BY run_id
                FOR UPDATE SKIP LOCKED
                LIMIT 1000
            );
            GET DIAGNOSTICS affected = ROW_COUNT;
            EXIT WHEN affected < 1000;
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$;
