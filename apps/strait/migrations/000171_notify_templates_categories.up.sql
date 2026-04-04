CREATE TABLE IF NOT EXISTS notification_templates (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id       TEXT NOT NULL,
    template_key     TEXT NOT NULL,
    name             TEXT NOT NULL,
    description      TEXT,
    version          INT NOT NULL DEFAULT 1,
    channels         JSONB NOT NULL,
    variables        JSONB NOT NULL DEFAULT '[]',
    locale_templates JSONB NOT NULL DEFAULT '{}',
    default_locale   TEXT NOT NULL DEFAULT 'en',
    status           TEXT NOT NULL DEFAULT 'active',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, template_key, version)
);

CREATE INDEX IF NOT EXISTS idx_notify_templates_project_status
    ON notification_templates(project_id, status);

CREATE TABLE IF NOT EXISTS notification_categories (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id   TEXT NOT NULL,
    category_key TEXT NOT NULL,
    name         TEXT NOT NULL,
    description  TEXT,
    type         TEXT NOT NULL DEFAULT 'product',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, category_key)
);
