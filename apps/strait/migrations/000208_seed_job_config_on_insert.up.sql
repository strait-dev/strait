-- R3 Phase 9 (fix-up for R2 Phase 6): the fan-out trigger from
-- migration 190 only fires on UPDATE jobs. Newly enqueued runs got
-- NULL job_enabled/job_paused/etc. because nothing populated them at
-- INSERT time. Add a BEFORE INSERT trigger that joins jobs and fills
-- in the denormalized columns.
--
-- This is forward-compatible with the R2 Phase 6 dequeue path which
-- COALESCEs the columns to (true, false) defaults; the trigger just
-- guarantees they always have the right values for the fully-
-- denormalized dequeue tests.

CREATE OR REPLACE FUNCTION seed_job_config_on_insert()
RETURNS trigger AS $$
BEGIN
    IF NEW.job_enabled IS NULL
       OR NEW.job_paused IS NULL
       OR NEW.job_max_concurrency IS NULL
       OR NEW.job_max_concurrency_per_key IS NULL THEN
        SELECT j.enabled, j.paused, j.max_concurrency, j.max_concurrency_per_key
        INTO NEW.job_enabled, NEW.job_paused, NEW.job_max_concurrency, NEW.job_max_concurrency_per_key
        FROM jobs j
        WHERE j.id = NEW.job_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS job_runs_seed_job_config_trg ON job_runs;
CREATE TRIGGER job_runs_seed_job_config_trg
BEFORE INSERT ON job_runs
FOR EACH ROW EXECUTE FUNCTION seed_job_config_on_insert();
