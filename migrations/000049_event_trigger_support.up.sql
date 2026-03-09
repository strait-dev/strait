-- Add event trigger fields to workflow_steps
ALTER TABLE workflow_steps ADD COLUMN event_key TEXT;
ALTER TABLE workflow_steps ADD COLUMN event_timeout_secs INT NOT NULL DEFAULT 3600;

-- Add event trigger fields to workflow_version_steps
ALTER TABLE workflow_version_steps ADD COLUMN event_key TEXT;
ALTER TABLE workflow_version_steps ADD COLUMN event_timeout_secs INT NOT NULL DEFAULT 3600;

-- Event triggers table for durable external event waits
CREATE TABLE event_triggers (
    id                    TEXT        PRIMARY KEY,
    event_key             TEXT        NOT NULL UNIQUE,
    project_id            TEXT        NOT NULL,
    source_type           TEXT        NOT NULL,
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
    error                 TEXT
);

CREATE INDEX idx_event_triggers_event_key ON event_triggers(event_key);
CREATE INDEX idx_event_triggers_status_expires ON event_triggers(status, expires_at)
    WHERE status = 'waiting' AND expires_at IS NOT NULL;
CREATE INDEX idx_event_triggers_step_run ON event_triggers(workflow_step_run_id)
    WHERE workflow_step_run_id IS NOT NULL;
CREATE INDEX idx_event_triggers_job_run ON event_triggers(job_run_id)
    WHERE job_run_id IS NOT NULL;
CREATE INDEX idx_event_triggers_project ON event_triggers(project_id, status);
