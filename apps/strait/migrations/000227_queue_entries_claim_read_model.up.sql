ALTER TABLE queue_entries
    ADD COLUMN IF NOT EXISTS run_status TEXT NOT NULL DEFAULT 'queued',
    ADD COLUMN IF NOT EXISTS job_enabled BOOLEAN,
    ADD COLUMN IF NOT EXISTS job_paused BOOLEAN,
    ADD COLUMN IF NOT EXISTS job_max_concurrency INT,
    ADD COLUMN IF NOT EXISTS job_max_concurrency_per_key INT,
    ADD COLUMN IF NOT EXISTS scheduled_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ;

UPDATE queue_entries qe
SET run_status = jr.status,
    job_enabled = jr.job_enabled,
    job_paused = jr.job_paused,
    job_max_concurrency = jr.job_max_concurrency,
    job_max_concurrency_per_key = jr.job_max_concurrency_per_key,
    scheduled_at = jr.scheduled_at,
    next_retry_at = jr.next_retry_at,
    updated_at = NOW()
FROM job_runs jr
WHERE jr.id = qe.run_id;

CREATE INDEX IF NOT EXISTS idx_queue_entries_claimable_denorm
    ON queue_entries(batch_id ASC, priority DESC, run_created_at ASC, run_id ASC)
    WHERE status = 'ready' AND run_status = 'queued';

CREATE INDEX IF NOT EXISTS idx_queue_entries_unbatched_denorm
    ON queue_entries(available_at ASC, run_created_at ASC, run_id ASC)
    WHERE status = 'ready' AND run_status = 'queued' AND batch_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_queue_entries_lease_expiry_denorm
    ON queue_entries(lease_expires_at ASC)
    WHERE status = 'leased' AND run_status = 'queued';

CREATE OR REPLACE FUNCTION queue_entry_sync_on_queued_status() RETURNS trigger AS $$
DECLARE
  claim_reset BOOLEAN;
BEGIN
  IF NEW.status <> 'queued' THEN
    UPDATE queue_entries
    SET run_status = NEW.status,
        job_id = NEW.job_id,
        project_id = NEW.project_id,
        priority = NEW.priority,
        run_created_at = COALESCE(NEW.created_at, NOW()),
        available_at = GREATEST(
            COALESCE(NEW.scheduled_at, '-infinity'::timestamptz),
            COALESCE(NEW.next_retry_at, '-infinity'::timestamptz),
            COALESCE(NEW.created_at, NOW())
        ),
        concurrency_key = COALESCE(NEW.concurrency_key, ''),
        job_enabled = NEW.job_enabled,
        job_paused = NEW.job_paused,
        job_max_concurrency = NEW.job_max_concurrency,
        job_max_concurrency_per_key = NEW.job_max_concurrency_per_key,
        scheduled_at = NEW.scheduled_at,
        next_retry_at = NEW.next_retry_at,
        updated_at = NOW()
    WHERE run_id = NEW.id;

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
      concurrency_key,
      run_status,
      job_enabled,
      job_paused,
      job_max_concurrency,
      job_max_concurrency_per_key,
      scheduled_at,
      next_retry_at
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
      COALESCE(NEW.concurrency_key, ''),
      NEW.status,
      NEW.job_enabled,
      NEW.job_paused,
      NEW.job_max_concurrency,
      NEW.job_max_concurrency_per_key,
      NEW.scheduled_at,
      NEW.next_retry_at
  )
  ON CONFLICT (run_id) DO UPDATE
  SET job_id = EXCLUDED.job_id,
      project_id = EXCLUDED.project_id,
      priority = EXCLUDED.priority,
      run_created_at = EXCLUDED.run_created_at,
      available_at = EXCLUDED.available_at,
      concurrency_key = EXCLUDED.concurrency_key,
      run_status = EXCLUDED.run_status,
      job_enabled = EXCLUDED.job_enabled,
      job_paused = EXCLUDED.job_paused,
      job_max_concurrency = EXCLUDED.job_max_concurrency,
      job_max_concurrency_per_key = EXCLUDED.job_max_concurrency_per_key,
      scheduled_at = EXCLUDED.scheduled_at,
      next_retry_at = EXCLUDED.next_retry_at,
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
AFTER INSERT OR UPDATE OF status, scheduled_at, next_retry_at, priority, job_id, project_id, created_at, concurrency_key, job_enabled, job_paused, job_max_concurrency, job_max_concurrency_per_key ON job_runs
FOR EACH ROW
EXECUTE FUNCTION queue_entry_sync_on_queued_status();
