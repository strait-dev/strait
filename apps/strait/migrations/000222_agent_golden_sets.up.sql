-- safety-ok: new table, no existing rows
CREATE TABLE agent_golden_sets (
    id          TEXT PRIMARY KEY DEFAULT gen_random_ulid(),
    agent_id    TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    project_id  TEXT NOT NULL,
    name        TEXT NOT NULL DEFAULT 'default',
    cases       JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (agent_id, name)
);

CREATE INDEX idx_agent_golden_sets_agent ON agent_golden_sets (agent_id);
