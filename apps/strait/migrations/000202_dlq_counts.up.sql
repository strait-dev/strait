-- Phase 9: DLQ counts table maintained by trigger so dlq depth and
-- configurable caps can be enforced with a single PK lookup, not a
-- SELECT COUNT(*) WHERE status='dead_letter' per failure.

CREATE TABLE IF NOT EXISTS dlq_counts (
    project_id TEXT NOT NULL,
    job_id     TEXT NOT NULL,
    count      INT  NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, job_id)
);

-- Seed from existing dead_letter rows so the cap is accurate on first
-- startup after the migration.
INSERT INTO dlq_counts (project_id, job_id, count)
SELECT project_id, job_id, COUNT(*)
FROM job_runs
WHERE status = 'dead_letter'
  AND (visible_until IS NULL OR visible_until > NOW())
GROUP BY project_id, job_id
ON CONFLICT (project_id, job_id) DO UPDATE SET count = EXCLUDED.count;

CREATE OR REPLACE FUNCTION dlq_counts_apply()
RETURNS trigger AS $$
DECLARE
    old_dlq BOOLEAN := FALSE;
    new_dlq BOOLEAN := FALSE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        new_dlq := NEW.status = 'dead_letter';
    ELSIF TG_OP = 'UPDATE' THEN
        old_dlq := OLD.status = 'dead_letter' AND (OLD.visible_until IS NULL OR OLD.visible_until > NOW());
        new_dlq := NEW.status = 'dead_letter' AND (NEW.visible_until IS NULL OR NEW.visible_until > NOW());
        -- Also drop the count when a row is soft-deleted (visible_until set).
        IF OLD.status = 'dead_letter' AND NEW.status = 'dead_letter'
           AND OLD.visible_until IS NULL AND NEW.visible_until IS NOT NULL THEN
            old_dlq := TRUE;
            new_dlq := FALSE;
        END IF;
    ELSIF TG_OP = 'DELETE' THEN
        old_dlq := OLD.status = 'dead_letter' AND (OLD.visible_until IS NULL OR OLD.visible_until > NOW());
    END IF;

    IF TG_OP IN ('UPDATE', 'DELETE') AND old_dlq THEN
        UPDATE dlq_counts
        SET count = GREATEST(count - 1, 0), updated_at = NOW()
        WHERE project_id = OLD.project_id AND job_id = OLD.job_id;
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') AND new_dlq THEN
        INSERT INTO dlq_counts (project_id, job_id, count)
        VALUES (NEW.project_id, NEW.job_id, 1)
        ON CONFLICT (project_id, job_id)
        DO UPDATE SET count = dlq_counts.count + 1, updated_at = NOW();
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS job_runs_dlq_counts_trg ON job_runs;
CREATE TRIGGER job_runs_dlq_counts_trg
AFTER INSERT OR UPDATE OR DELETE ON job_runs
FOR EACH ROW EXECUTE FUNCTION dlq_counts_apply();
