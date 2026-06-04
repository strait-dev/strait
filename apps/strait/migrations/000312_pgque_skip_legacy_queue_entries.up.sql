CREATE OR REPLACE FUNCTION queue_entry_sync_on_queued_status() RETURNS trigger AS $$
DECLARE
  claim_reset BOOLEAN;
  job_environment_id TEXT;
BEGIN
  IF current_setting('strait.queue_backend', true) = 'pgque' THEN
    RETURN NEW;
  END IF;

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
