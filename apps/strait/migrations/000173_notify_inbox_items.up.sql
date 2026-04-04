CREATE TABLE IF NOT EXISTS inbox_items (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    recipient_type  TEXT NOT NULL,
    recipient_id    TEXT NOT NULL,
    project_id      TEXT NOT NULL,
    tenant_id       TEXT,
    workflow_id     TEXT,
    workflow_run_id TEXT,
    category_key    TEXT,
    title           TEXT NOT NULL,
    body            TEXT,
    avatar          TEXT,
    priority        TEXT NOT NULL DEFAULT 'normal',
    state           TEXT NOT NULL DEFAULT 'unread',
    actions         JSONB NOT NULL DEFAULT '[]',
    dedup_key       TEXT,
    dedup_count     INT NOT NULL DEFAULT 1,
    read_at         TIMESTAMPTZ,
    archived_at     TIMESTAMPTZ,
    actioned_at     TIMESTAMPTZ,
    action_result   JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notify_inbox_items_recipient
    ON inbox_items(recipient_type, recipient_id, state, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notify_inbox_items_project
    ON inbox_items(project_id, state, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notify_inbox_items_dedup
    ON inbox_items(project_id, recipient_id, dedup_key)
    WHERE dedup_key IS NOT NULL AND state != 'archived';
