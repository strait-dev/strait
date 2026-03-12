CREATE TABLE IF NOT EXISTS webhook_subscriptions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    webhook_url TEXT NOT NULL,
    event_types TEXT[] NOT NULL DEFAULT '{}',
    secret TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_project_active
    ON webhook_subscriptions(project_id, created_at DESC)
    WHERE active = TRUE;
