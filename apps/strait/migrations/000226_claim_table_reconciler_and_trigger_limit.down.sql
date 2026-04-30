-- Restore unbounded fan-out trigger.
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
