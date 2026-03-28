ALTER TABLE workflow_step_runs
    ALTER COLUMN workflow_step_id DROP NOT NULL;

CREATE TABLE workflow_dynamic_steps (
    id                 TEXT        PRIMARY KEY DEFAULT uuidv7(),
    workflow_run_id    TEXT        NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    parent_step_run_id TEXT        NOT NULL REFERENCES workflow_step_runs(id) ON DELETE CASCADE,
    step_ref           TEXT        NOT NULL,
    depends_on         TEXT[]      NOT NULL DEFAULT '{}',
    definition         JSONB       NOT NULL,
    dynamic_depth      INT         NOT NULL DEFAULT 1,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workflow_run_id, step_ref),
    CHECK (dynamic_depth > 0)
);

CREATE INDEX idx_workflow_dynamic_steps_run
    ON workflow_dynamic_steps (workflow_run_id, created_at);

CREATE INDEX idx_workflow_dynamic_steps_parent
    ON workflow_dynamic_steps (parent_step_run_id);
