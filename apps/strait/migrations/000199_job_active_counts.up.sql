-- Phase 6: job_active_counts table maintained via trigger so the dequeue
-- hot path does not have to GROUP BY / COUNT over every in-flight row.
--
-- The existing dequeue CTE scans all rows in status IN ('dequeued',
-- 'executing') per-claim. At 10k+ active runs this is a full index scan
-- per dequeue attempt on every worker. This table lets the dequeue look
-- up the active count with a single PK lookup instead.
--
-- Ownership: maintained by a trigger on job_runs. The trigger increments
-- the count on transition INTO ('dequeued','executing') and decrements on
-- transition OUT. Idempotency guaranteed by the transaction: both the
-- job_runs UPDATE and the counter UPDATE commit together.

CREATE TABLE IF NOT EXISTS job_active_counts (
    job_id          TEXT NOT NULL,
    concurrency_key TEXT NOT NULL DEFAULT '',
    count           INT  NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (job_id, concurrency_key)
);

-- Seed existing state so enabling the denormalized dequeue path does not
-- start from zero. If job_runs has nothing active this is a no-op.
INSERT INTO job_active_counts (job_id, concurrency_key, count)
SELECT job_id, COALESCE(concurrency_key, ''), COUNT(*)
FROM job_runs
WHERE status IN ('dequeued', 'executing')
GROUP BY job_id, COALESCE(concurrency_key, '')
ON CONFLICT (job_id, concurrency_key) DO UPDATE SET count = EXCLUDED.count;

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

DROP TRIGGER IF EXISTS job_runs_active_counts_trg ON job_runs;
CREATE TRIGGER job_runs_active_counts_trg
AFTER INSERT OR UPDATE OR DELETE ON job_runs
FOR EACH ROW EXECUTE FUNCTION job_active_counts_apply();
