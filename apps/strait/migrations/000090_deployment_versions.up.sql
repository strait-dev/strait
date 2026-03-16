CREATE TABLE deployment_versions (
    id                          TEXT PRIMARY KEY,
    project_id                  TEXT        NOT NULL,
    environment                 TEXT        NOT NULL,
    runtime                     TEXT        NOT NULL,
    artifact_uri                TEXT        NOT NULL,
    manifest                    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    checksum                    TEXT        NOT NULL DEFAULT '',
    status                      TEXT        NOT NULL DEFAULT 'draft',
    finalized_at                TIMESTAMPTZ NULL,
    promoted_at                 TIMESTAMPTZ NULL,
    rollback_from_deployment_id TEXT        NULL REFERENCES deployment_versions(id) ON DELETE SET NULL,
    created_by                  TEXT        NOT NULL DEFAULT '',
    updated_by                  TEXT        NOT NULL DEFAULT '',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT deployment_versions_status_check CHECK (status IN ('draft', 'finalized', 'promoted'))
);

CREATE INDEX idx_deployment_versions_project_env_created
    ON deployment_versions(project_id, environment, created_at DESC);

CREATE UNIQUE INDEX idx_deployment_versions_single_promoted
    ON deployment_versions(project_id, environment)
    WHERE promoted_at IS NOT NULL;