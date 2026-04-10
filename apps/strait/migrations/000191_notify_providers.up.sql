CREATE TABLE IF NOT EXISTS notification_providers (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id  TEXT NOT NULL,
    channel     TEXT NOT NULL,
    provider    TEXT NOT NULL,
    name        TEXT NOT NULL,
    config_enc  BYTEA NOT NULL,
    is_default  BOOLEAN NOT NULL DEFAULT FALSE,
    fallback_id TEXT,
    health      TEXT NOT NULL DEFAULT 'healthy',
    rate_limit  INT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, channel, provider)
);
