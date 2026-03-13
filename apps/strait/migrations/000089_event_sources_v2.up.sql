CREATE TABLE event_sources (
    id          TEXT        PRIMARY KEY,
    project_id  TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    schema      JSONB,
    enabled     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, name)
);

CREATE INDEX idx_event_sources_project ON event_sources(project_id, enabled);

CREATE TABLE event_subscriptions (
    id          TEXT        PRIMARY KEY,
    source_id   TEXT        NOT NULL REFERENCES event_sources(id) ON DELETE CASCADE,
    target_type TEXT        NOT NULL,
    target_id   TEXT        NOT NULL,
    filter_expr JSONB,
    enabled     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source_id, target_type, target_id)
);

CREATE INDEX idx_event_subscriptions_source ON event_subscriptions(source_id, enabled);
CREATE INDEX idx_event_subscriptions_target ON event_subscriptions(target_type, target_id);
