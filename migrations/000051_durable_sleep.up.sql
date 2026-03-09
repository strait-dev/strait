-- Durable sleep step support
ALTER TABLE event_triggers ADD COLUMN trigger_type TEXT NOT NULL DEFAULT 'event';

-- Sleep duration for workflow steps
ALTER TABLE workflow_steps ADD COLUMN sleep_duration_secs INT NOT NULL DEFAULT 0;
ALTER TABLE workflow_version_steps ADD COLUMN sleep_duration_secs INT NOT NULL DEFAULT 0;
