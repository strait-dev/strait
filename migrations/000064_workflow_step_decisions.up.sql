CREATE TABLE workflow_step_decisions (
    id              TEXT PRIMARY KEY,
    workflow_run_id TEXT NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    step_run_id     TEXT NOT NULL REFERENCES workflow_step_runs(id) ON DELETE CASCADE,
    step_ref        TEXT NOT NULL,
    decision_type   TEXT NOT NULL,
    decision        TEXT NOT NULL,
    explanation     TEXT NOT NULL,
    details         JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflow_step_decisions_run_created
    ON workflow_step_decisions(workflow_run_id, created_at DESC);
CREATE INDEX idx_workflow_step_decisions_step
    ON workflow_step_decisions(workflow_run_id, step_ref, created_at DESC);
