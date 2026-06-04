-- job_run_state is now the authoritative hot-path state table. Keep active
-- concurrency counters attached to it so PgQue, legacy queue sync, and direct
-- state transitions share one source of truth without scanning active rows.
CREATE OR REPLACE FUNCTION job_active_counts_apply()
RETURNS trigger AS $$
DECLARE
    old_active BOOLEAN := FALSE;
    new_active BOOLEAN := FALSE;
    old_key TEXT := '';
    new_key TEXT := '';
BEGIN
    IF TG_OP = 'INSERT' THEN
        new_active := NEW.status IN ('dequeued', 'executing');
        new_key := COALESCE(NEW.concurrency_key, '');
    ELSIF TG_OP = 'UPDATE' THEN
        old_active := OLD.status IN ('dequeued', 'executing');
        new_active := NEW.status IN ('dequeued', 'executing');
        old_key := COALESCE(OLD.concurrency_key, '');
        new_key := COALESCE(NEW.concurrency_key, '');
    ELSIF TG_OP = 'DELETE' THEN
        old_active := OLD.status IN ('dequeued', 'executing');
        old_key := COALESCE(OLD.concurrency_key, '');
    END IF;

    IF TG_OP IN ('UPDATE', 'DELETE') AND old_active THEN
        UPDATE job_active_counts
        SET count = GREATEST(count - 1, 0), updated_at = NOW()
        WHERE job_id = OLD.job_id AND concurrency_key = old_key;
    END IF;

    IF TG_OP IN ('INSERT', 'UPDATE') AND new_active THEN
        INSERT INTO job_active_counts (job_id, concurrency_key, count)
        VALUES (NEW.job_id, new_key, 1)
        ON CONFLICT (job_id, concurrency_key)
        DO UPDATE SET count = job_active_counts.count + 1, updated_at = NOW();
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DELETE FROM job_active_counts;

INSERT INTO job_active_counts (job_id, concurrency_key, count)
SELECT job_id, COALESCE(concurrency_key, ''), COUNT(*)
FROM job_run_state
WHERE status IN ('dequeued', 'executing')
GROUP BY job_id, COALESCE(concurrency_key, '')
ON CONFLICT (job_id, concurrency_key) DO UPDATE SET count = EXCLUDED.count;

DROP TRIGGER IF EXISTS job_runs_active_counts_trg ON job_runs;
DROP TRIGGER IF EXISTS job_run_state_active_counts_trg ON job_run_state;
CREATE TRIGGER job_run_state_active_counts_trg
AFTER INSERT OR UPDATE OR DELETE ON job_run_state
FOR EACH ROW EXECUTE FUNCTION job_active_counts_apply();

-- PgQue receives explicit run IDs from PgQue batches and claims by run_id.
-- Broad partial claim indexes whose predicate depends on status make every
-- queued->active->terminal state transition update indexes and block HOT.
DROP INDEX IF EXISTS idx_job_run_state_claim_http;
DROP INDEX IF EXISTS idx_job_run_state_project_claim;
DROP INDEX IF EXISTS idx_job_run_state_worker_claim;
