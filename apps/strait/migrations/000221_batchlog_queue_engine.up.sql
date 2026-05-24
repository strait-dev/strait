CREATE TABLE IF NOT EXISTS queue_batches (
    id BIGSERIAL PRIMARY KEY,
    sealed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sealed_until TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS queue_batch_ticks (
    id BIGSERIAL PRIMARY KEY,
    tick_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sealed_until TIMESTAMPTZ NOT NULL,
    batch_id BIGINT REFERENCES queue_batches(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS queue_entries (
    run_id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    priority INT NOT NULL DEFAULT 0,
    run_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    batch_id BIGINT REFERENCES queue_batches(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'ready' CHECK (status IN ('ready', 'leased', 'acked')),
    lease_owner TEXT,
    lease_expires_at TIMESTAMPTZ,
    claimed_at TIMESTAMPTZ,
    acked_at TIMESTAMPTZ,
    attempts INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_queue_entries_claimable
    ON queue_entries(batch_id, priority DESC, run_created_at ASC)
    WHERE status = 'ready';

CREATE INDEX IF NOT EXISTS idx_queue_entries_lease_expiry
    ON queue_entries(lease_expires_at)
    WHERE status = 'leased';

CREATE INDEX IF NOT EXISTS idx_queue_entries_unbatched
    ON queue_entries(available_at, run_created_at)
    WHERE status = 'ready' AND batch_id IS NULL;

CREATE OR REPLACE FUNCTION queue_entry_ack_on_run_status() RETURNS trigger AS $$
BEGIN
  IF NEW.status IN ('executing', 'completed', 'failed', 'timed_out', 'crashed', 'canceled', 'expired', 'dead_letter', 'system_failed') THEN
    UPDATE queue_entries
    SET status = 'acked',
        acked_at = COALESCE(acked_at, NOW()),
        lease_owner = NULL,
        lease_expires_at = NULL,
        updated_at = NOW()
    WHERE run_id = NEW.id
      AND status IN ('ready', 'leased');
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_queue_entry_ack_on_run_status ON job_runs;

CREATE TRIGGER trg_queue_entry_ack_on_run_status
AFTER UPDATE OF status ON job_runs
FOR EACH ROW
EXECUTE FUNCTION queue_entry_ack_on_run_status();
