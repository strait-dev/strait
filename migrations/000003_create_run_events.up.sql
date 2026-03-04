CREATE TABLE run_events (
    id         TEXT        PRIMARY KEY,
    run_id     TEXT        NOT NULL REFERENCES job_runs(id),
    type       TEXT        NOT NULL,
    level      TEXT,
    message    TEXT,
    data       JSONB       NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_events_run_id ON run_events(run_id, created_at ASC);
