CREATE TABLE project_quotas (
    project_id          TEXT        PRIMARY KEY,
    max_queued_runs     INT,
    max_executing_runs  INT,
    max_jobs            INT,
    timezone            TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
