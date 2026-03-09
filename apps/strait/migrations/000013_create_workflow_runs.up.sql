CREATE TABLE workflow_runs (
    id            TEXT        PRIMARY KEY,
    workflow_id   TEXT        NOT NULL REFERENCES workflows(id),
    project_id    TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'pending',
    triggered_by  TEXT        NOT NULL DEFAULT 'manual',
    payload       JSONB,
    error         TEXT,
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_workflow_runs_workflow_id ON workflow_runs(workflow_id);
CREATE INDEX idx_workflow_runs_project_status ON workflow_runs(project_id, status);

CREATE TABLE workflow_step_runs (
    id                TEXT        PRIMARY KEY,
    workflow_run_id   TEXT        NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    workflow_step_id  TEXT        NOT NULL REFERENCES workflow_steps(id),
    step_ref          TEXT        NOT NULL,
    job_run_id        TEXT,
    status            TEXT        NOT NULL DEFAULT 'pending',
    deps_completed    INT         NOT NULL DEFAULT 0,
    deps_required     INT         NOT NULL DEFAULT 0,
    output            JSONB,
    error             TEXT,
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workflow_run_id, step_ref)
);
CREATE INDEX idx_step_runs_workflow_run ON workflow_step_runs(workflow_run_id);
CREATE INDEX idx_step_runs_job_run ON workflow_step_runs(job_run_id) WHERE job_run_id IS NOT NULL;

ALTER TABLE job_runs ADD COLUMN workflow_step_run_id TEXT;
CREATE INDEX idx_job_runs_workflow_step_run ON job_runs(workflow_step_run_id) WHERE workflow_step_run_id IS NOT NULL;
