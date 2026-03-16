-- No FK to job_runs: job_runs is partitioned (PK is composite id+created_at),
-- so single-column FK references are not supported. Cleanup is handled by
-- the retention reaper which deletes terminal runs and cascades via application logic.
CREATE TABLE run_state (
  run_id TEXT NOT NULL,
  state_key TEXT NOT NULL CHECK (length(state_key) <= 256),
  value JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (run_id, state_key)
);
