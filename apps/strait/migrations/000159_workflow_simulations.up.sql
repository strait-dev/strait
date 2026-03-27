-- Workflow simulation history: per-user simulation runs.
CREATE TABLE IF NOT EXISTS workflow_simulations (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    project_id      TEXT NOT NULL,
    user_id         TEXT NOT NULL,
    mode            TEXT NOT NULL DEFAULT 'dry_run',
    payload         JSONB,
    failure_inject  JSONB,
    result          JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workflow_simulations_user
    ON workflow_simulations (user_id, workflow_id, created_at DESC);
