ALTER TABLE jobs ADD COLUMN debounce_window_secs INT NOT NULL DEFAULT 0;

CREATE TABLE debounce_pending (
  id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  debounce_key TEXT NOT NULL DEFAULT '',
  payload JSONB,
  tags JSONB,
  priority INT NOT NULL DEFAULT 0,
  concurrency_key TEXT,
  ttl_secs INT,
  triggered_by TEXT NOT NULL DEFAULT 'api',
  created_by TEXT,
  fire_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(job_id, debounce_key)
);
CREATE INDEX idx_debounce_pending_fire ON debounce_pending(fire_at);
