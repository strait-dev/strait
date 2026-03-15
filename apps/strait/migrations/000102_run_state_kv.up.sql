CREATE TABLE run_state (
  run_id TEXT NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
  state_key TEXT NOT NULL CHECK (length(state_key) <= 256),
  value JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (run_id, state_key)
);
