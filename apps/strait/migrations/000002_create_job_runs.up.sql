CREATE TABLE job_runs (
    id             TEXT        PRIMARY KEY,
    job_id         TEXT        NOT NULL REFERENCES jobs(id),
    project_id     TEXT        NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'queued',
    attempt        INT         NOT NULL DEFAULT 1,
    payload        JSONB,
    result         JSONB,
    error          TEXT,
    triggered_by   TEXT        NOT NULL DEFAULT 'manual',
    scheduled_at   TIMESTAMPTZ,
    started_at     TIMESTAMPTZ,
    finished_at    TIMESTAMPTZ,
    heartbeat_at   TIMESTAMPTZ,
    next_retry_at  TIMESTAMPTZ,
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_runs_queue ON job_runs(created_at ASC) WHERE status = 'queued';
CREATE INDEX idx_runs_project_status ON job_runs(project_id, status, created_at DESC);
CREATE INDEX idx_runs_heartbeat ON job_runs(heartbeat_at) WHERE status = 'executing';
CREATE INDEX idx_runs_expires ON job_runs(expires_at) WHERE expires_at IS NOT NULL AND status IN ('delayed', 'queued');
