-- Bound the fan-out trigger to prevent unbounded UPDATE statements when a
-- job has a very large number of queued/delayed runs. The LIMIT 10000
-- ensures the trigger completes in bounded time; a NOTICE is raised when
-- the limit is hit so operators can investigate or run the update manually.
CREATE OR REPLACE FUNCTION fanout_job_config_to_runs()
RETURNS trigger AS $$
DECLARE
    affected_count INT;
BEGIN
    IF NEW.enabled IS DISTINCT FROM OLD.enabled
       OR NEW.paused IS DISTINCT FROM OLD.paused
       OR NEW.max_concurrency IS DISTINCT FROM OLD.max_concurrency
       OR NEW.max_concurrency_per_key IS DISTINCT FROM OLD.max_concurrency_per_key THEN
        WITH bounded AS (
            SELECT id FROM job_runs
            WHERE job_id = NEW.id
              AND status IN ('queued', 'delayed')
            LIMIT 10000
        )
        UPDATE job_runs
        SET job_enabled = NEW.enabled,
            job_paused = NEW.paused,
            job_max_concurrency = NEW.max_concurrency,
            job_max_concurrency_per_key = NEW.max_concurrency_per_key
        WHERE id IN (SELECT id FROM bounded);

        GET DIAGNOSTICS affected_count = ROW_COUNT;
        IF affected_count >= 10000 THEN
            RAISE NOTICE 'fanout_job_config_to_runs: hit 10000-row limit for job_id=%, remaining rows need manual update', NEW.id;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
