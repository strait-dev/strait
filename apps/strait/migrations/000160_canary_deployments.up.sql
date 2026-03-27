-- Canary deployments: traffic ramping between workflow versions.
CREATE TABLE IF NOT EXISTS canary_deployments (
    id                  TEXT PRIMARY KEY,
    workflow_id         TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    project_id          TEXT NOT NULL,
    source_version      INT NOT NULL,
    target_version      INT NOT NULL,
    traffic_pct         INT NOT NULL DEFAULT 0 CHECK (traffic_pct >= 0 AND traffic_pct <= 100),
    status              TEXT NOT NULL DEFAULT 'active',
    auto_promote_config JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_canary_deployments_active_workflow
    ON canary_deployments (workflow_id)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_canary_deployments_project
    ON canary_deployments (project_id, status);
