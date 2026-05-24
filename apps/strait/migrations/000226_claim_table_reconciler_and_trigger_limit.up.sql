-- Fix the jobs fan-out trigger to batch-UPDATE claim rows in chunks of 1000
-- instead of a single unbounded UPDATE. For a job with 100K pending runs,
-- the original trigger would do a 100K-row UPDATE in one shot, blocking the
-- transaction and creating 100K dead tuples on the claim table.
--
-- The replacement uses a loop with LIMIT 1000 per iteration.

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
    THEN
        LOOP
            UPDATE job_run_queue
            SET job_enabled = NEW.enabled,
                job_paused = NEW.paused,
                job_max_concurrency = NEW.max_concurrency,
                job_max_concurrency_per_key = NEW.max_concurrency_per_key
			WHERE run_id IN (
				SELECT run_id FROM job_run_queue
				WHERE job_id = NEW.id
				  AND (job_enabled IS DISTINCT FROM NEW.enabled
				       OR job_paused IS DISTINCT FROM NEW.paused
				       OR job_max_concurrency IS DISTINCT FROM NEW.max_concurrency
				       OR job_max_concurrency_per_key IS DISTINCT FROM NEW.max_concurrency_per_key)
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
