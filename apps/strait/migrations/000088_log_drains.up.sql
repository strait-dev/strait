CREATE TABLE log_drains (
    id           TEXT        PRIMARY KEY,
    project_id   TEXT        NOT NULL,
    name         TEXT        NOT NULL,
    drain_type   TEXT        NOT NULL DEFAULT 'http',
    endpoint_url TEXT        NOT NULL,
    auth_type    TEXT        NOT NULL DEFAULT 'none',
    auth_config  JSONB       NOT NULL DEFAULT '{}',
    level_filter TEXT[]      NOT NULL DEFAULT '{}',
    enabled      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_log_drains_project ON log_drains(project_id, enabled);
