CREATE TABLE IF NOT EXISTS job_batchlog_lease_counts (
    job_id TEXT NOT NULL,
    concurrency_key TEXT NOT NULL DEFAULT '',
    count INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (job_id, concurrency_key)
);

INSERT INTO job_batchlog_lease_counts (job_id, concurrency_key, count)
SELECT qe.job_id, COALESCE(qe.concurrency_key, ''), COUNT(*)::int
FROM queue_entries qe
WHERE qe.status = 'leased'
  AND qe.run_status = 'queued'
GROUP BY qe.job_id, COALESCE(qe.concurrency_key, '')
ON CONFLICT (job_id, concurrency_key) DO UPDATE
SET count = EXCLUDED.count,
    updated_at = NOW();

CREATE OR REPLACE FUNCTION job_batchlog_lease_counts_apply()
RETURNS trigger AS $$
DECLARE
    old_leased BOOLEAN := FALSE;
    new_leased BOOLEAN := FALSE;
    old_key TEXT := '';
    new_key TEXT := '';
BEGIN
    IF TG_OP = 'INSERT' THEN
        new_leased := NEW.status = 'leased';
        new_key := COALESCE(NEW.concurrency_key, '');
    ELSIF TG_OP = 'UPDATE' THEN
        old_leased := OLD.status = 'leased';
        new_leased := NEW.status = 'leased';
        old_key := COALESCE(OLD.concurrency_key, '');
        new_key := COALESCE(NEW.concurrency_key, '');
    ELSIF TG_OP = 'DELETE' THEN
        old_leased := OLD.status = 'leased';
        old_key := COALESCE(OLD.concurrency_key, '');
    END IF;

    IF TG_OP IN ('UPDATE', 'DELETE') AND old_leased THEN
        UPDATE job_batchlog_lease_counts
        SET count = GREATEST(count - 1, 0),
            updated_at = NOW()
        WHERE job_id = OLD.job_id
          AND concurrency_key = old_key;
    END IF;

    IF TG_OP IN ('INSERT', 'UPDATE') AND new_leased THEN
        INSERT INTO job_batchlog_lease_counts (job_id, concurrency_key, count)
        VALUES (NEW.job_id, new_key, 1)
        ON CONFLICT (job_id, concurrency_key)
        DO UPDATE SET count = job_batchlog_lease_counts.count + 1,
                      updated_at = NOW();
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS queue_entries_lease_counts_trg ON queue_entries;
CREATE TRIGGER queue_entries_lease_counts_trg
AFTER INSERT OR UPDATE OR DELETE ON queue_entries
FOR EACH ROW EXECUTE FUNCTION job_batchlog_lease_counts_apply();

DROP INDEX IF EXISTS idx_queue_entries_leased_key_denorm;
