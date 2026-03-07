CREATE TABLE run_usage (
    id                  TEXT        PRIMARY KEY,
    run_id              TEXT        NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    provider            TEXT        NOT NULL,
    model               TEXT        NOT NULL,
    prompt_tokens       INT         NOT NULL DEFAULT 0,
    completion_tokens   INT         NOT NULL DEFAULT 0,
    total_tokens        INT         NOT NULL DEFAULT 0,
    cost_microusd       BIGINT      NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_run_usage_run_id_created_at ON run_usage(run_id, created_at DESC);
