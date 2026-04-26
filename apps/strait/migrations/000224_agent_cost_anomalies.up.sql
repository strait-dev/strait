-- safety-ok: new table, no existing rows
CREATE TABLE agent_cost_anomalies (
    id                    TEXT PRIMARY KEY DEFAULT gen_random_ulid(),
    agent_id              TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    project_id            TEXT NOT NULL,
    detected_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    daily_cost_microusd   BIGINT NOT NULL,
    baseline_avg_microusd BIGINT NOT NULL,
    multiplier            DOUBLE PRECISION NOT NULL,
    threshold             DOUBLE PRECISION NOT NULL,
    status                TEXT NOT NULL DEFAULT 'open',
    resolved_at           TIMESTAMPTZ,
    snoozed_until         TIMESTAMPTZ
);

CREATE INDEX idx_agent_cost_anomalies_agent ON agent_cost_anomalies (agent_id, detected_at DESC);
