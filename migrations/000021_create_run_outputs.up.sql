CREATE TABLE run_outputs (
    id                TEXT        PRIMARY KEY,
    run_id            TEXT        NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    output_key        TEXT        NOT NULL,
    schema            JSONB,
    value             JSONB       NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, output_key)
);

CREATE INDEX idx_run_outputs_run_id ON run_outputs(run_id);
