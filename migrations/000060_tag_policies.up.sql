CREATE TABLE IF NOT EXISTS tag_policies (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    user_id       TEXT NOT NULL,
    tag_key       TEXT NOT NULL,
    tag_value     TEXT,
    actions       TEXT[] NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, resource_type, user_id, tag_key, tag_value)
);

CREATE INDEX IF NOT EXISTS idx_tag_policies_lookup
    ON tag_policies(project_id, resource_type, user_id);
