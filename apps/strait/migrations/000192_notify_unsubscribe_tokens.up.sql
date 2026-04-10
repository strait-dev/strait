CREATE TABLE IF NOT EXISTS unsubscribe_tokens (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id    TEXT NOT NULL,
    subscriber_id TEXT NOT NULL,
    scope         TEXT NOT NULL,
    token         TEXT NOT NULL UNIQUE,
    used_at       TIMESTAMPTZ,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notify_unsubscribe_token
    ON unsubscribe_tokens(token)
    WHERE used_at IS NULL;
