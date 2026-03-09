ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS event_emit_key;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS event_emit_key;
