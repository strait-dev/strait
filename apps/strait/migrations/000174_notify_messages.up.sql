CREATE TABLE IF NOT EXISTS notification_messages (
    id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id         TEXT NOT NULL,
    idempotency_key    TEXT,
    recipient_type     TEXT NOT NULL,
    recipient_id       TEXT NOT NULL,
    tenant_id          TEXT,
    workflow_run_id    TEXT,
    step_run_id        TEXT,
    template_id        TEXT,
    category_key       TEXT,
    channel            TEXT NOT NULL,
    provider_id        TEXT,
    rendered_content   JSONB,
    ai_generated       BOOLEAN NOT NULL DEFAULT FALSE,
    status             TEXT NOT NULL DEFAULT 'rendering',
    attempts           INT NOT NULL DEFAULT 0,
    provider_response  JSONB,
    delivered_at       TIMESTAMPTZ,
    read_at            TIMESTAMPTZ,
    clicked_at         TIMESTAMPTZ,
    bounced_at         TIMESTAMPTZ,
    suppression_reason TEXT,
    batch_id           TEXT,
    scheduled_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notify_messages_status
    ON notification_messages(status, created_at);

CREATE INDEX IF NOT EXISTS idx_notify_messages_recipient
    ON notification_messages(recipient_type, recipient_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notify_messages_project
    ON notification_messages(project_id, status, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notify_messages_idempotency
    ON notification_messages(project_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_notify_messages_batch
    ON notification_messages(batch_id)
    WHERE batch_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_notify_messages_scheduled
    ON notification_messages(scheduled_at)
    WHERE status = 'scheduled';
