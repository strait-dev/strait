CREATE TABLE IF NOT EXISTS queue_batch_seal_state (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    last_sealed_until TIMESTAMPTZ NOT NULL DEFAULT '-infinity'::timestamptz,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- safety-ok: queue_entries is a narrow queue-side table; golang-migrate runs this migration in a transaction, so CONCURRENTLY cannot be used here.
CREATE INDEX IF NOT EXISTS idx_queue_entries_claimable_batch_order
    ON queue_entries(batch_id ASC, priority DESC, run_created_at ASC, run_id ASC)
    WHERE status = 'ready';

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
      status
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
      'ready'
  )
  ON CONFLICT (run_id) DO UPDATE
  SET job_id = EXCLUDED.job_id,
      project_id = EXCLUDED.project_id,
      priority = EXCLUDED.priority,
      run_created_at = EXCLUDED.run_created_at,
      available_at = EXCLUDED.available_at,
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
AFTER INSERT OR UPDATE OF status, scheduled_at, next_retry_at, priority, job_id, project_id, created_at ON job_runs
FOR EACH ROW
EXECUTE FUNCTION queue_entry_sync_on_queued_status();

INSERT INTO queue_entries (
    run_id,
    job_id,
    project_id,
    priority,
    run_created_at,
    available_at,
    status
)
SELECT
    jr.id,
    jr.job_id,
    jr.project_id,
    jr.priority,
    jr.created_at,
    GREATEST(
        COALESCE(jr.scheduled_at, '-infinity'::timestamptz),
        COALESCE(jr.next_retry_at, '-infinity'::timestamptz),
        jr.created_at
    ),
    'ready'
FROM job_runs jr
WHERE jr.status = 'queued'
ON CONFLICT (run_id) DO NOTHING;
