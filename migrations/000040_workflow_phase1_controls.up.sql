ALTER TABLE workflows
    ADD COLUMN timeout_secs INT NOT NULL DEFAULT 0,
    ADD COLUMN max_concurrent_runs INT NOT NULL DEFAULT 0;

ALTER TABLE workflow_steps
    ADD COLUMN step_type TEXT NOT NULL DEFAULT 'job',
    ADD COLUMN approval_timeout_secs INT NOT NULL DEFAULT 0,
    ADD COLUMN approval_approvers TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN retry_max_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN retry_backoff TEXT NOT NULL DEFAULT 'exponential',
    ADD COLUMN retry_initial_delay_secs INT NOT NULL DEFAULT 1,
    ADD COLUMN retry_max_delay_secs INT NOT NULL DEFAULT 3600;

ALTER TABLE workflow_runs
    ADD COLUMN workflow_version INT NOT NULL DEFAULT 1;

ALTER TABLE job_runs
    ADD COLUMN max_attempts_override INT,
    ADD COLUMN timeout_secs_override INT,
    ADD COLUMN retry_backoff TEXT,
    ADD COLUMN retry_initial_delay_secs INT,
    ADD COLUMN retry_max_delay_secs INT;

CREATE TABLE workflow_versions (
    id                   TEXT        PRIMARY KEY,
    workflow_id          TEXT        NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    version              INT         NOT NULL,
    project_id           TEXT        NOT NULL,
    name                 TEXT        NOT NULL,
    slug                 TEXT        NOT NULL,
    description          TEXT,
    enabled              BOOLEAN     NOT NULL,
    timeout_secs         INT         NOT NULL,
    max_concurrent_runs  INT         NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workflow_id, version)
);

CREATE INDEX idx_workflow_versions_workflow_id ON workflow_versions(workflow_id);

CREATE TABLE workflow_version_steps (
    id                        TEXT        PRIMARY KEY,
    workflow_version_id       TEXT        NOT NULL REFERENCES workflow_versions(id) ON DELETE CASCADE,
    job_id                    TEXT        REFERENCES jobs(id),
    step_ref                  TEXT        NOT NULL,
    depends_on                TEXT[]      NOT NULL DEFAULT '{}',
    condition                 JSONB,
    on_failure                TEXT        NOT NULL DEFAULT 'fail_workflow',
    payload                   JSONB,
    step_type                 TEXT        NOT NULL DEFAULT 'job',
    approval_timeout_secs     INT         NOT NULL DEFAULT 0,
    approval_approvers        TEXT[]      NOT NULL DEFAULT '{}',
    retry_max_attempts        INT         NOT NULL DEFAULT 0,
    retry_backoff             TEXT        NOT NULL DEFAULT 'exponential',
    retry_initial_delay_secs  INT         NOT NULL DEFAULT 1,
    retry_max_delay_secs      INT         NOT NULL DEFAULT 3600,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workflow_version_id, step_ref)
);

CREATE INDEX idx_workflow_version_steps_version_id ON workflow_version_steps(workflow_version_id);

CREATE TABLE workflow_step_approvals (
    id                    TEXT        PRIMARY KEY,
    workflow_run_id       TEXT        NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    workflow_step_run_id  TEXT        NOT NULL REFERENCES workflow_step_runs(id) ON DELETE CASCADE,
    approvers             TEXT[]      NOT NULL DEFAULT '{}',
    status                TEXT        NOT NULL DEFAULT 'pending',
    approved_by           TEXT,
    requested_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_at           TIMESTAMPTZ,
    expires_at            TIMESTAMPTZ,
    error                 TEXT,
    UNIQUE (workflow_step_run_id)
);

CREATE INDEX idx_workflow_runs_workflow_status_created ON workflow_runs(workflow_id, status, created_at);
CREATE INDEX idx_workflow_step_approvals_status_expires ON workflow_step_approvals(status, expires_at);
