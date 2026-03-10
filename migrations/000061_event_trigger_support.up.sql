-- Event trigger fields on workflow_steps
ALTER TABLE workflow_steps
  ADD COLUMN event_key TEXT,
  ADD COLUMN event_timeout_secs INT NOT NULL DEFAULT 3600,
  ADD COLUMN event_notify_url TEXT,
  ADD COLUMN event_emit_key TEXT,
  ADD COLUMN sleep_duration_secs INT NOT NULL DEFAULT 0;

-- Mirror on versioned steps
ALTER TABLE workflow_version_steps
  ADD COLUMN event_key TEXT,
  ADD COLUMN event_timeout_secs INT NOT NULL DEFAULT 3600,
  ADD COLUMN event_notify_url TEXT,
  ADD COLUMN event_emit_key TEXT,
  ADD COLUMN sleep_duration_secs INT NOT NULL DEFAULT 0;

-- Event triggers table
CREATE TABLE event_triggers (
    id                    TEXT        PRIMARY KEY,
    event_key             TEXT        NOT NULL UNIQUE,
    project_id            TEXT        NOT NULL,
    source_type           TEXT        NOT NULL,
    trigger_type          TEXT        NOT NULL DEFAULT 'event',
    workflow_run_id       TEXT,
    workflow_step_run_id  TEXT,
    job_run_id            TEXT,
    status                TEXT        NOT NULL DEFAULT 'waiting',
    request_payload       JSONB,
    response_payload      JSONB,
    timeout_secs          INT         NOT NULL DEFAULT 3600,
    requested_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    received_at           TIMESTAMPTZ,
    expires_at            TIMESTAMPTZ NOT NULL,
    error                 TEXT,
    notify_url            TEXT,
    notify_status         TEXT        NOT NULL DEFAULT '',
    event_emit_key        TEXT,
    sent_by               TEXT        NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Reaper: find expired waiting triggers
CREATE INDEX idx_event_triggers_status_expires
    ON event_triggers(status, expires_at) WHERE status = 'waiting' AND expires_at IS NOT NULL;

-- Lookup by step/job/workflow run
CREATE INDEX idx_event_triggers_step_run
    ON event_triggers(workflow_step_run_id) WHERE workflow_step_run_id IS NOT NULL;
CREATE INDEX idx_event_triggers_job_run
    ON event_triggers(job_run_id) WHERE job_run_id IS NOT NULL;
CREATE INDEX idx_event_triggers_project
    ON event_triggers(project_id, status);

-- Cancel triggers when workflow is canceled/timed out
CREATE INDEX idx_event_triggers_workflow_run
    ON event_triggers(workflow_run_id, status) WHERE status = 'waiting';

-- Reconciliation: find received triggers with stale steps
CREATE INDEX idx_event_triggers_reconcile
    ON event_triggers(status, source_type, received_at) WHERE status = 'received';

-- Prefix matching for batch send
CREATE INDEX idx_event_triggers_event_key_prefix
    ON event_triggers(event_key text_pattern_ops);

-- Extend webhook_deliveries for event trigger notifications
ALTER TABLE webhook_deliveries
  ADD COLUMN IF NOT EXISTS event_trigger_id TEXT REFERENCES event_triggers(id) ON DELETE CASCADE;
ALTER TABLE webhook_deliveries ALTER COLUMN run_id DROP NOT NULL;
ALTER TABLE webhook_deliveries ALTER COLUMN job_id DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending_retry
    ON webhook_deliveries(next_retry_at) WHERE status = 'pending' AND next_retry_at IS NOT NULL;
