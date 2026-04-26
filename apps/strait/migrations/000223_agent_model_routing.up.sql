-- safety-ok: new table, no existing rows
CREATE TABLE agent_model_routing (
    id              TEXT PRIMARY KEY DEFAULT gen_random_ulid(),
    agent_id        TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tier            TEXT NOT NULL CHECK (tier IN ('simple', 'standard', 'complex')),
    model           TEXT NOT NULL,
    quality_score   DOUBLE PRECISION,
    previous_model  TEXT NOT NULL DEFAULT '',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by      TEXT NOT NULL DEFAULT '',
    UNIQUE (agent_id, tier)
);
