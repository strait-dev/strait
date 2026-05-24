ALTER TABLE queue_entries
    ADD COLUMN IF NOT EXISTS concurrency_key TEXT NOT NULL DEFAULT '';

UPDATE queue_entries qe
SET concurrency_key = COALESCE(jr.concurrency_key, ''),
    updated_at = NOW()
FROM job_runs jr
WHERE jr.id = qe.run_id
  AND qe.concurrency_key <> COALESCE(jr.concurrency_key, '');

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
JOIN job_runs jr ON jr.id = qe.run_id
WHERE qe.status = 'leased'
  AND jr.status = 'queued'
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

CREATE OR REPLACE FUNCTION queue_entry_sync_on_queued_status() RETURNS trigger AS $$
DECLARE
  claim_reset BOOLEAN;
BEGIN
  IF NEW.status <> 'queued' THEN
    RETURN NEW;
  END IF;

  IF TG_OP = 'INSERT' THEN
    claim_reset := TRUE;
  ELSE
    claim_reset := OLD.status IS DISTINCT FROM NEW.status;
  END IF;

  INSERT INTO queue_entries (
      run_id,
      job_id,
      project_id,
      priority,
      run_created_at,
      available_at,
      status,
      concurrency_key
  )
  VALUES (
      NEW.id,
      NEW.job_id,
      NEW.project_id,
      NEW.priority,
      COALESCE(NEW.created_at, NOW()),
      GREATEST(
          COALESCE(NEW.scheduled_at, '-infinity'::timestamptz),
          COALESCE(NEW.next_retry_at, '-infinity'::timestamptz),
          COALESCE(NEW.created_at, NOW())
      ),
      'ready',
      COALESCE(NEW.concurrency_key, '')
  )
  ON CONFLICT (run_id) DO UPDATE
  SET job_id = EXCLUDED.job_id,
      project_id = EXCLUDED.project_id,
      priority = EXCLUDED.priority,
      run_created_at = EXCLUDED.run_created_at,
      available_at = EXCLUDED.available_at,
      concurrency_key = EXCLUDED.concurrency_key,
      status = CASE
          WHEN NOT claim_reset
           AND queue_entries.status = 'leased'
           AND queue_entries.lease_expires_at > NOW()
          THEN queue_entries.status
          ELSE 'ready'
      END,
      batch_id = CASE
          WHEN NOT claim_reset
           AND queue_entries.status = 'leased'
           AND queue_entries.lease_expires_at > NOW()
          THEN queue_entries.batch_id
          ELSE NULL
      END,
      lease_owner = CASE
          WHEN NOT claim_reset
           AND queue_entries.status = 'leased'
           AND queue_entries.lease_expires_at > NOW()
          THEN queue_entries.lease_owner
          ELSE NULL
      END,
      lease_expires_at = CASE
          WHEN NOT claim_reset
           AND queue_entries.status = 'leased'
           AND queue_entries.lease_expires_at > NOW()
          THEN queue_entries.lease_expires_at
          ELSE NULL
      END,
      acked_at = NULL,
      updated_at = NOW();

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_queue_entry_sync_on_queued_status ON job_runs;

CREATE TRIGGER trg_queue_entry_sync_on_queued_status
AFTER INSERT OR UPDATE OF status, scheduled_at, next_retry_at, priority, job_id, project_id, created_at, concurrency_key ON job_runs
FOR EACH ROW
EXECUTE FUNCTION queue_entry_sync_on_queued_status();
