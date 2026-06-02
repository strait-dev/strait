CREATE OR REPLACE FUNCTION job_active_counts_apply()
RETURNS trigger AS $$
DECLARE
    old_active BOOLEAN := FALSE;
    new_active BOOLEAN := FALSE;
    old_key TEXT := '';
    new_key TEXT := '';
BEGIN
    IF TG_OP = 'INSERT' THEN
        new_active := NEW.status IN ('dequeued', 'executing')
            AND (NEW.job_max_concurrency IS NOT NULL OR NEW.job_max_concurrency_per_key IS NOT NULL);
        new_key := COALESCE(NEW.concurrency_key, '');
    ELSIF TG_OP = 'UPDATE' THEN
        old_active := OLD.status IN ('dequeued', 'executing')
            AND (OLD.job_max_concurrency IS NOT NULL OR OLD.job_max_concurrency_per_key IS NOT NULL);
        new_active := NEW.status IN ('dequeued', 'executing')
            AND (NEW.job_max_concurrency IS NOT NULL OR NEW.job_max_concurrency_per_key IS NOT NULL);
        old_key := COALESCE(OLD.concurrency_key, '');
        new_key := COALESCE(NEW.concurrency_key, '');

        IF old_active
           AND new_active
           AND OLD.job_id = NEW.job_id
           AND old_key = new_key
        THEN
            RETURN NEW;
        END IF;
    ELSIF TG_OP = 'DELETE' THEN
        old_active := OLD.status IN ('dequeued', 'executing')
            AND (OLD.job_max_concurrency IS NOT NULL OR OLD.job_max_concurrency_per_key IS NOT NULL);
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
  AND (job_max_concurrency IS NOT NULL OR job_max_concurrency_per_key IS NOT NULL)
GROUP BY job_id, COALESCE(concurrency_key, '')
ON CONFLICT (job_id, concurrency_key) DO UPDATE SET count = EXCLUDED.count;
