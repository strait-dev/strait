-- safety-ok: new table, no existing rows
CREATE TABLE agent_autopilot_actions (
    id              TEXT PRIMARY KEY DEFAULT gen_random_ulid(),
    agent_id        TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tier            TEXT NOT NULL,
    previous_model  TEXT NOT NULL,
    new_model       TEXT NOT NULL,
    budget_pct      DOUBLE PRECISION NOT NULL,
    quality_score   DOUBLE PRECISION,
    action          TEXT NOT NULL CHECK (action IN ('downgrade', 'revert', 'skip')),
    reason          TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_autopilot_actions_agent ON agent_autopilot_actions (agent_id, created_at DESC);
