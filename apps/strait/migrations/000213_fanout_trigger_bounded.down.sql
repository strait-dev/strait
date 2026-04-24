-- Restore the original unbounded fan-out trigger from migration 000190.
CREATE OR REPLACE FUNCTION fanout_job_config_to_runs()
RETURNS trigger AS $$
BEGIN
    IF NEW.enabled IS DISTINCT FROM OLD.enabled
       OR NEW.paused IS DISTINCT FROM OLD.paused
       OR NEW.max_concurrency IS DISTINCT FROM OLD.max_concurrency
       OR NEW.max_concurrency_per_key IS DISTINCT FROM OLD.max_concurrency_per_key THEN
        UPDATE job_runs
        SET job_enabled = NEW.enabled,
            job_paused = NEW.paused,
            job_max_concurrency = NEW.max_concurrency,
            job_max_concurrency_per_key = NEW.max_concurrency_per_key
        WHERE job_id = NEW.id
          AND status IN ('queued', 'delayed');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
