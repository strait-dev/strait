CREATE TABLE notification_channels (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id   TEXT NOT NULL,
    channel_type TEXT NOT NULL CHECK (channel_type IN ('slack', 'discord', 'webhook', 'email')),
    name         TEXT NOT NULL,
    config       BYTEA NOT NULL,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_notification_channels_project ON notification_channels(project_id, enabled);

CREATE TABLE notification_deliveries (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    channel_id    TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    project_id    TEXT NOT NULL,
    event_type    TEXT NOT NULL,
    payload       JSONB NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL DEFAULT 'pending',
    attempts      INT NOT NULL DEFAULT 0,
    max_attempts  INT NOT NULL DEFAULT 3,
    last_error    TEXT,
    next_retry_at TIMESTAMPTZ,
    delivered_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_notification_deliveries_pending ON notification_deliveries(status, next_retry_at) WHERE status = 'pending';
