CREATE TABLE IF NOT EXISTS notification_preferences (
    id                  TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    recipient_type      TEXT NOT NULL,
    recipient_id        TEXT NOT NULL,
    scope               TEXT NOT NULL DEFAULT 'global',
    channel_prefs       JSONB NOT NULL DEFAULT '{}',
    quiet_hours         JSONB,
    phone               TEXT,
    timezone            TEXT NOT NULL DEFAULT 'UTC',
    digest_policy       TEXT NOT NULL DEFAULT 'immediate',
    critical_override   BOOLEAN NOT NULL DEFAULT TRUE,
    rate_limit_override INT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (recipient_type, recipient_id, scope)
);
