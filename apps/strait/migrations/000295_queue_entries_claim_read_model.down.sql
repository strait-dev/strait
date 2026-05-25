DROP TRIGGER IF EXISTS trg_queue_entry_sync_on_queued_status ON job_runs;

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

CREATE TRIGGER trg_queue_entry_sync_on_queued_status
AFTER INSERT OR UPDATE OF status, scheduled_at, next_retry_at, priority, job_id, project_id, created_at, concurrency_key ON job_runs
FOR EACH ROW
EXECUTE FUNCTION queue_entry_sync_on_queued_status();

DROP INDEX IF EXISTS idx_queue_entries_lease_expiry_denorm;
DROP INDEX IF EXISTS idx_queue_entries_unbatched_denorm;
DROP INDEX IF EXISTS idx_queue_entries_claimable_denorm;

ALTER TABLE queue_entries
    DROP COLUMN IF EXISTS next_retry_at,
    DROP COLUMN IF EXISTS scheduled_at,
    DROP COLUMN IF EXISTS job_max_concurrency_per_key,
    DROP COLUMN IF EXISTS job_max_concurrency,
    DROP COLUMN IF EXISTS job_paused,
    DROP COLUMN IF EXISTS job_enabled,
    DROP COLUMN IF EXISTS run_status;
