ALTER TABLE jobs
  ADD COLUMN batch_window_secs INT NOT NULL DEFAULT 0,
  ADD COLUMN batch_max_size INT NOT NULL DEFAULT 0;

CREATE TABLE batch_buffer (
  id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  batch_key TEXT NOT NULL DEFAULT '',
  payload JSONB NOT NULL,
  tags JSONB,
  priority INT NOT NULL DEFAULT 0,
  triggered_by TEXT NOT NULL DEFAULT 'api',
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_batch_buffer_job_key ON batch_buffer(job_id, batch_key);
CREATE INDEX idx_batch_buffer_created ON batch_buffer(created_at);
