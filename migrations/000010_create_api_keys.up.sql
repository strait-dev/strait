CREATE TABLE api_keys (
    id           TEXT        PRIMARY KEY,
    project_id   TEXT        NOT NULL,
    name         TEXT        NOT NULL,
    key_hash     TEXT        NOT NULL,
    key_prefix   TEXT        NOT NULL,
    scopes       TEXT[]      NOT NULL DEFAULT '{}',
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at   TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX idx_api_keys_project_id ON api_keys(project_id);
