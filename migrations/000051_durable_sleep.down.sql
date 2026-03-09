ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS sleep_duration_secs;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS sleep_duration_secs;
ALTER TABLE event_triggers DROP COLUMN IF EXISTS trigger_type;
