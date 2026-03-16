-- Compute metering: track container wall-clock time and cost per run.
CREATE TABLE IF NOT EXISTS run_compute_usage (
    id               TEXT PRIMARY KEY,
    run_id           TEXT NOT NULL,
    project_id       TEXT NOT NULL,
    job_id           TEXT NOT NULL,
    machine_preset   TEXT NOT NULL,
    machine_id       TEXT NOT NULL DEFAULT '',
    duration_secs    DOUBLE PRECISION NOT NULL DEFAULT 0,
    cost_microusd    BIGINT NOT NULL DEFAULT 0,
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_run_compute_usage_run_id ON run_compute_usage (run_id);
CREATE INDEX IF NOT EXISTS idx_run_compute_usage_project_day ON run_compute_usage (project_id, created_at);

-- Add compute_daily_cost_limit_microusd to project_quotas for budget enforcement.
ALTER TABLE project_quotas ADD COLUMN IF NOT EXISTS compute_daily_cost_limit_microusd BIGINT;
