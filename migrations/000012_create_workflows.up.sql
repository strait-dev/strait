CREATE TABLE workflows (
    id            TEXT        PRIMARY KEY,
    project_id    TEXT        NOT NULL,
    name          TEXT        NOT NULL,
    slug          TEXT        NOT NULL,
    description   TEXT,
    enabled       BOOLEAN     NOT NULL DEFAULT TRUE,
    version       INT         NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, slug)
);
CREATE INDEX idx_workflows_project_id ON workflows(project_id);

CREATE TABLE workflow_steps (
    id             TEXT        PRIMARY KEY,
    workflow_id    TEXT        NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    job_id         TEXT        NOT NULL REFERENCES jobs(id),
    step_ref       TEXT        NOT NULL,
    depends_on     TEXT[]      NOT NULL DEFAULT '{}',
    condition      JSONB,
    on_failure     TEXT        NOT NULL DEFAULT 'fail_workflow',
    payload        JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workflow_id, step_ref)
);
CREATE INDEX idx_workflow_steps_workflow_id ON workflow_steps(workflow_id);
