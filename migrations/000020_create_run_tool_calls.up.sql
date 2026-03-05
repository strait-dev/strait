CREATE TABLE run_tool_calls (
    id                TEXT        PRIMARY KEY,
    run_id            TEXT        NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    tool_name         TEXT        NOT NULL,
    input             JSONB,
    output            JSONB,
    duration_ms       INT,
    status            TEXT        NOT NULL DEFAULT 'completed',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_run_tool_calls_run_id_created_at ON run_tool_calls(run_id, created_at DESC);
