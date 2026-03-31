CREATE TABLE agents (
    id           TEXT PRIMARY KEY,
    project_id   TEXT        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    job_id       TEXT        NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    slug         TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    model        TEXT        NOT NULL,
    config       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_by   TEXT,
    updated_by   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, slug),
    UNIQUE (job_id)
);

CREATE INDEX idx_agents_project_created_at
    ON agents (project_id, created_at DESC);

CREATE TABLE agent_deployments (
    id                TEXT PRIMARY KEY,
    agent_id          TEXT        NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    version           INT         NOT NULL,
    status            TEXT        NOT NULL,
    provider          TEXT        NOT NULL,
    config_snapshot   JSONB       NOT NULL DEFAULT '{}'::jsonb,
    provider_metadata JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_by        TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deployed_at       TIMESTAMPTZ,
    UNIQUE (agent_id, version)
);

CREATE INDEX idx_agent_deployments_agent_created_at
    ON agent_deployments (agent_id, created_at DESC);
