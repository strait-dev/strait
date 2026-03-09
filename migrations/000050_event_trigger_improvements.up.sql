-- Notification webhook support for event triggers
ALTER TABLE event_triggers ADD COLUMN notify_url TEXT;
ALTER TABLE event_triggers ADD COLUMN notify_status TEXT NOT NULL DEFAULT '';

-- Notification webhook URL for workflow wait_for_event steps
ALTER TABLE workflow_steps ADD COLUMN event_notify_url TEXT;
ALTER TABLE workflow_version_steps ADD COLUMN event_notify_url TEXT;

-- Index for cancellation queries
CREATE INDEX idx_event_triggers_workflow_run ON event_triggers(workflow_run_id, status)
    WHERE status = 'waiting';

-- Index for reconciliation queries
CREATE INDEX idx_event_triggers_reconcile ON event_triggers(status, source_type, received_at)
    WHERE status = 'received';
