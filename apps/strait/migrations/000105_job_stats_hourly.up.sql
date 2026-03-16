CREATE TABLE job_stats_hourly (
  job_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  hour TIMESTAMPTZ NOT NULL,
  total INT NOT NULL DEFAULT 0,
  completed INT NOT NULL DEFAULT 0,
  failed INT NOT NULL DEFAULT 0,
  timed_out INT NOT NULL DEFAULT 0,
  canceled INT NOT NULL DEFAULT 0,
  avg_duration_ms BIGINT,
  p95_duration_ms BIGINT,
  total_cost_microusd BIGINT DEFAULT 0,
  PRIMARY KEY (job_id, hour)
);

CREATE INDEX idx_job_stats_hourly_project ON job_stats_hourly(project_id, hour);
