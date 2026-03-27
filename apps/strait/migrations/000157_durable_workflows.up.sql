-- Durable workflow support: expected completion tracking, step duration estimates, stage notifications.
ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS expected_completion_at TIMESTAMPTZ;

ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS expected_duration_secs INT NOT NULL DEFAULT 0;
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS stage_notifications JSONB;

ALTER TABLE workflow_version_steps ADD COLUMN IF NOT EXISTS expected_duration_secs INT NOT NULL DEFAULT 0;
ALTER TABLE workflow_version_steps ADD COLUMN IF NOT EXISTS stage_notifications JSONB;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_runs_expected_completion
    ON workflow_runs (expected_completion_at)
    WHERE status = 'running';
