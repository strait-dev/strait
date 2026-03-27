DROP INDEX IF EXISTS idx_workflow_runs_expected_completion;
ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS stage_notifications;
ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS expected_duration_secs;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS stage_notifications;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS expected_duration_secs;
ALTER TABLE workflow_runs DROP COLUMN IF EXISTS expected_completion_at;
