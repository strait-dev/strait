-- R2 Phase 6: denormalize job config onto job_runs so the dequeue hot path
-- no longer needs JOIN jobs at all. The Phase 6 (Round 1) DequeueNDenormalized
-- still reads the job row for enabled/paused/max_concurrency; we now copy
-- those onto every new run row and maintain them via a fan-out trigger on
-- jobs UPDATE.
--
-- This is additive: existing callers continue to work. The new columns are
-- populated by the trigger so seeding is automatic. Behavioural change only
-- when the feature flag QUEUE_USE_DENORMALIZED_DEQUEUE is enabled and the
-- executor opts into the fully-denormalized dequeue path.

ALTER TABLE job_runs
    ADD COLUMN IF NOT EXISTS job_enabled              BOOLEAN,
    ADD COLUMN IF NOT EXISTS job_paused               BOOLEAN,
    ADD COLUMN IF NOT EXISTS job_max_concurrency      INT,
    ADD COLUMN IF NOT EXISTS job_max_concurrency_per_key INT;

-- Seed from existing rows (only active statuses matter for dequeue hot path).
UPDATE job_runs jr
SET job_enabled = j.enabled,
    job_paused = j.paused,
    job_max_concurrency = j.max_concurrency,
    job_max_concurrency_per_key = j.max_concurrency_per_key
FROM jobs j
WHERE j.id = jr.job_id
  AND jr.status IN ('queued', 'delayed');

-- Fan-out trigger on jobs updates: when enabled/paused/max_concurrency
-- change, push the new value onto any non-terminal runs for that job.
-- Bounded to status IN ('queued','delayed') so terminal rows stay frozen.
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

DROP TRIGGER IF EXISTS jobs_fanout_config_trg ON jobs;
CREATE TRIGGER jobs_fanout_config_trg
AFTER UPDATE ON jobs
FOR EACH ROW EXECUTE FUNCTION fanout_job_config_to_runs();
