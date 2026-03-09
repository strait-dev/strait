DROP TABLE IF EXISTS event_triggers;

ALTER TABLE workflow_steps DROP COLUMN IF EXISTS event_key;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS event_timeout_secs;

ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS event_key;
ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS event_timeout_secs;
