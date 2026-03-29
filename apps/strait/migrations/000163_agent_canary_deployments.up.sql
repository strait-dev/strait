CREATE TABLE agent_canary_deployments (
    id                     TEXT PRIMARY KEY,
    agent_id               TEXT        NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    project_id             TEXT        NOT NULL,
    source_deployment_id   TEXT        NOT NULL REFERENCES agent_deployments(id),
    target_deployment_id   TEXT        NOT NULL REFERENCES agent_deployments(id),
    traffic_pct            INT         NOT NULL DEFAULT 0 CHECK (traffic_pct >= 0 AND traffic_pct <= 100),
    status                 TEXT        NOT NULL DEFAULT 'active',
    auto_promote_config    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at           TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_agent_canary_active
    ON agent_canary_deployments (agent_id) WHERE status = 'active';

CREATE INDEX idx_agent_canary_agent
    ON agent_canary_deployments (agent_id, created_at DESC);
