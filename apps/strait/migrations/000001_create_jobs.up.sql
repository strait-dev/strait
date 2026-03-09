CREATE TABLE jobs (
    id            TEXT        PRIMARY KEY,
    project_id    TEXT        NOT NULL,
    name          TEXT        NOT NULL,
    slug          TEXT        NOT NULL,
    description   TEXT,
    cron          TEXT,
    payload_schema JSONB,
    endpoint_url  TEXT        NOT NULL,
    max_attempts  INT         NOT NULL DEFAULT 3,
    timeout_secs  INT         NOT NULL DEFAULT 300,
    enabled       BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, slug)
);
CREATE INDEX idx_jobs_project_id ON jobs(project_id);
