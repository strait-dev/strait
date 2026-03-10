CREATE TABLE run_checkpoints (
    id            TEXT        PRIMARY KEY,
    run_id        TEXT        NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    sequence      INT         NOT NULL,
    source        TEXT        NOT NULL DEFAULT 'auto',
    state         JSONB       NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, sequence)
);

CREATE INDEX idx_run_checkpoints_run_id_sequence ON run_checkpoints(run_id, sequence DESC);
