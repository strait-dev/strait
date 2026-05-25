ALTER TABLE queue_entries
    ADD COLUMN IF NOT EXISTS execution_mode TEXT,
    ADD COLUMN IF NOT EXISTS queue_name TEXT,
    ADD COLUMN IF NOT EXISTS environment_id TEXT;

UPDATE queue_entries qe
SET execution_mode = COALESCE(NULLIF(jr.execution_mode, ''), 'http'),
    queue_name = COALESCE(NULLIF(jr.queue_name, ''), 'default'),
    environment_id = COALESCE(j.environment_id, ''),
    updated_at = NOW()
FROM job_runs jr
JOIN jobs j ON j.id = jr.job_id
WHERE jr.id = qe.run_id;

ALTER TABLE queue_entries
    ALTER COLUMN execution_mode SET DEFAULT 'http',
    ALTER COLUMN queue_name SET DEFAULT 'default',
    ALTER COLUMN environment_id SET DEFAULT '';

-- safety-ok: queue_entries is a narrow queue-side table; golang-migrate runs this migration in a transaction, so CONCURRENTLY cannot be used here.
CREATE INDEX IF NOT EXISTS idx_queue_entries_claimable_http_denorm
    ON queue_entries(batch_id ASC, priority DESC, run_created_at ASC, run_id ASC)
    WHERE status = 'ready' AND run_status = 'queued' AND execution_mode = 'http';

-- safety-ok: queue_entries is a narrow queue-side table; golang-migrate runs this migration in a transaction, so CONCURRENTLY cannot be used here.
CREATE INDEX IF NOT EXISTS idx_queue_entries_claimable_worker_denorm
    ON queue_entries(project_id, queue_name, environment_id, batch_id ASC, priority DESC, run_created_at ASC, run_id ASC)
    WHERE status = 'ready' AND run_status = 'queued' AND execution_mode = 'worker';

-- safety-ok: queue_entries is a narrow queue-side table; golang-migrate runs this migration in a transaction, so CONCURRENTLY cannot be used here.
CREATE INDEX IF NOT EXISTS idx_queue_entries_acked_cleanup
    ON queue_entries(acked_at ASC, run_id ASC)
    WHERE status = 'acked';

-- safety-ok: workflow_progression_events is an internal queue-side table; golang-migrate runs this migration in a transaction, so CONCURRENTLY cannot be used here.
CREATE INDEX IF NOT EXISTS idx_workflow_progression_events_processed_cleanup
    ON workflow_progression_events(processed_at ASC, id ASC)
    WHERE processed_at IS NOT NULL;

CREATE OR REPLACE FUNCTION queue_entry_sync_on_queued_status() RETURNS trigger AS $$
DECLARE
  claim_reset BOOLEAN;
  job_environment_id TEXT;
BEGIN
  SELECT COALESCE(j.environment_id, '')
  INTO job_environment_id
  FROM jobs j
  WHERE j.id = NEW.job_id;

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
        execution_mode = COALESCE(NULLIF(NEW.execution_mode, ''), 'http'),
        queue_name = COALESCE(NULLIF(NEW.queue_name, ''), 'default'),
        environment_id = COALESCE(job_environment_id, ''),
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
      next_retry_at,
      execution_mode,
      queue_name,
      environment_id
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
      NEW.next_retry_at,
      COALESCE(NULLIF(NEW.execution_mode, ''), 'http'),
      COALESCE(NULLIF(NEW.queue_name, ''), 'default'),
      COALESCE(job_environment_id, '')
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
      execution_mode = EXCLUDED.execution_mode,
      queue_name = EXCLUDED.queue_name,
      environment_id = EXCLUDED.environment_id,
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
AFTER INSERT OR UPDATE OF status, scheduled_at, next_retry_at, priority, job_id, project_id, created_at, concurrency_key, job_enabled, job_paused, job_max_concurrency, job_max_concurrency_per_key, execution_mode, queue_name ON job_runs
FOR EACH ROW
EXECUTE FUNCTION queue_entry_sync_on_queued_status();

CREATE OR REPLACE FUNCTION trg_jobs_fanout_to_queue()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public, pg_catalog
AS $$
DECLARE
    affected int;
BEGIN
    IF NEW.enabled IS DISTINCT FROM OLD.enabled
       OR NEW.paused IS DISTINCT FROM OLD.paused
       OR NEW.max_concurrency IS DISTINCT FROM OLD.max_concurrency
       OR NEW.max_concurrency_per_key IS DISTINCT FROM OLD.max_concurrency_per_key
       OR NEW.queue_name IS DISTINCT FROM OLD.queue_name
       OR NEW.execution_mode IS DISTINCT FROM OLD.execution_mode
       OR NEW.environment_id IS DISTINCT FROM OLD.environment_id
    THEN
        LOOP
            UPDATE job_run_queue
            SET job_enabled = NEW.enabled,
                job_paused = NEW.paused,
                job_max_concurrency = NEW.max_concurrency,
                job_max_concurrency_per_key = NEW.max_concurrency_per_key,
                queue_name = NEW.queue_name,
                execution_mode = NEW.execution_mode
            WHERE run_id IN (
                SELECT run_id FROM job_run_queue
                WHERE job_id = NEW.id
                  AND (job_enabled IS DISTINCT FROM NEW.enabled
                       OR job_paused IS DISTINCT FROM NEW.paused
                       OR job_max_concurrency IS DISTINCT FROM NEW.max_concurrency
                       OR job_max_concurrency_per_key IS DISTINCT FROM NEW.max_concurrency_per_key
                       OR queue_name IS DISTINCT FROM NEW.queue_name
                       OR execution_mode IS DISTINCT FROM NEW.execution_mode)
                ORDER BY run_id
                FOR UPDATE SKIP LOCKED
                LIMIT 1000
            );
            GET DIAGNOSTICS affected = ROW_COUNT;
            EXIT WHEN affected < 1000;
        END LOOP;

        LOOP
            UPDATE queue_entries
            SET job_enabled = NEW.enabled,
                job_paused = NEW.paused,
                job_max_concurrency = NEW.max_concurrency,
                job_max_concurrency_per_key = NEW.max_concurrency_per_key,
                queue_name = COALESCE(NULLIF(NEW.queue_name, ''), 'default'),
                execution_mode = COALESCE(NULLIF(NEW.execution_mode, ''), 'http'),
                environment_id = COALESCE(NEW.environment_id, ''),
                updated_at = NOW()
            WHERE run_id IN (
                SELECT run_id FROM queue_entries
                WHERE job_id = NEW.id
                  AND status IN ('ready', 'leased')
                  AND (job_enabled IS DISTINCT FROM NEW.enabled
                       OR job_paused IS DISTINCT FROM NEW.paused
                       OR job_max_concurrency IS DISTINCT FROM NEW.max_concurrency
                       OR job_max_concurrency_per_key IS DISTINCT FROM NEW.max_concurrency_per_key
                       OR queue_name IS DISTINCT FROM COALESCE(NULLIF(NEW.queue_name, ''), 'default')
                       OR execution_mode IS DISTINCT FROM COALESCE(NULLIF(NEW.execution_mode, ''), 'http')
                       OR environment_id IS DISTINCT FROM COALESCE(NEW.environment_id, ''))
                ORDER BY run_id
                FOR UPDATE SKIP LOCKED
                LIMIT 1000
            );
            GET DIAGNOSTICS affected = ROW_COUNT;
            EXIT WHEN affected < 1000;
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$;
