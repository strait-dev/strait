CREATE TABLE IF NOT EXISTS subscribers (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id  TEXT NOT NULL,
    external_id TEXT NOT NULL,
    email       TEXT,
    phone       TEXT,
    locale      TEXT NOT NULL DEFAULT 'en',
    timezone    TEXT NOT NULL DEFAULT 'UTC',
    push_tokens JSONB NOT NULL DEFAULT '[]',
    attributes  JSONB NOT NULL DEFAULT '{}',
    tenant_id   TEXT,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, external_id)
);

CREATE INDEX IF NOT EXISTS idx_notify_subscribers_project_status
    ON subscribers(project_id, status);

CREATE INDEX IF NOT EXISTS idx_notify_subscribers_tenant
    ON subscribers(project_id, tenant_id)
    WHERE tenant_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS topics (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id  TEXT NOT NULL,
    topic_key   TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT,
    attributes  JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, topic_key)
);

CREATE TABLE IF NOT EXISTS topic_memberships (
    topic_id       TEXT NOT NULL,
    subscriber_id  TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (topic_id, subscriber_id)
);

CREATE INDEX IF NOT EXISTS idx_notify_topic_memberships_subscriber
    ON topic_memberships(subscriber_id);
