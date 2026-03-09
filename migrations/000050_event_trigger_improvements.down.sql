DROP INDEX IF EXISTS idx_event_triggers_reconcile;
DROP INDEX IF EXISTS idx_event_triggers_workflow_run;
ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS event_notify_url;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS event_notify_url;
ALTER TABLE event_triggers DROP COLUMN IF EXISTS notify_status;
ALTER TABLE event_triggers DROP COLUMN IF EXISTS notify_url;
