CREATE TABLE workflow_policies (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    max_fan_out     INT NOT NULL DEFAULT 0,
    max_depth       INT NOT NULL DEFAULT 0,
    forbidden_step_types TEXT[] NOT NULL DEFAULT '{}',
    require_approval_for_deploy BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id)
);
