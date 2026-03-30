CREATE TABLE agent_messages (
    id                TEXT PRIMARY KEY,
    project_id        TEXT        NOT NULL,
    source_agent_id   TEXT        NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    target_agent_id   TEXT        NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    source_run_id     TEXT,
    chain_id          TEXT        NOT NULL,
    chain_depth       INT         NOT NULL DEFAULT 1 CHECK (chain_depth > 0 AND chain_depth <= 20),
    payload           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    status            TEXT        NOT NULL DEFAULT 'pending',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at      TIMESTAMPTZ,
    error             TEXT
);

CREATE INDEX idx_agent_messages_target
    ON agent_messages (target_agent_id, status, created_at);

CREATE INDEX idx_agent_messages_chain
    ON agent_messages (chain_id);

CREATE INDEX idx_agent_messages_project
    ON agent_messages (project_id, created_at DESC);
