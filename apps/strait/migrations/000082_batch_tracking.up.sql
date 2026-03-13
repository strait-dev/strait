CREATE TABLE batch_operations (
    id           TEXT        PRIMARY KEY,
    project_id   TEXT        NOT NULL,
    job_id       TEXT        NOT NULL REFERENCES jobs(id),
    item_count   INT         NOT NULL,
    created_count INT        NOT NULL DEFAULT 0,
    created_by   TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at  TIMESTAMPTZ
);

CREATE INDEX idx_batch_operations_project
    ON batch_operations(project_id, created_at DESC);

ALTER TABLE job_runs
    ADD COLUMN IF NOT EXISTS batch_id TEXT REFERENCES batch_operations(id);

CREATE INDEX IF NOT EXISTS idx_job_runs_batch_id
    ON job_runs(batch_id)
    WHERE batch_id IS NOT NULL;
