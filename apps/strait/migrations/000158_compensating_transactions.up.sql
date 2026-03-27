-- Compensating transactions: rollback handlers on workflow steps.
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS compensation_job_id TEXT REFERENCES jobs(id);
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS compensation_timeout_secs INT NOT NULL DEFAULT 30;

ALTER TABLE workflow_version_steps ADD COLUMN IF NOT EXISTS compensation_job_id TEXT REFERENCES jobs(id);
ALTER TABLE workflow_version_steps ADD COLUMN IF NOT EXISTS compensation_timeout_secs INT NOT NULL DEFAULT 30;

CREATE TABLE IF NOT EXISTS compensation_runs (
    id                  TEXT PRIMARY KEY,
    workflow_run_id     TEXT NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    step_run_id         TEXT NOT NULL,
    step_ref            TEXT NOT NULL,
    compensation_job_id TEXT NOT NULL REFERENCES jobs(id),
    job_run_id          TEXT,
    status              TEXT NOT NULL DEFAULT 'pending',
    input               JSONB,
    output              JSONB,
    error               TEXT,
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_compensation_runs_workflow_run
    ON compensation_runs (workflow_run_id);
