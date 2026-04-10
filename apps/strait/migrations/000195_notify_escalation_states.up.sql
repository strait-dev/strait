CREATE TABLE IF NOT EXISTS escalation_states (
    id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id         TEXT NOT NULL,
    step_run_id        TEXT NOT NULL,
    workflow_run_id    TEXT NOT NULL,
    current_tier       INT NOT NULL DEFAULT 0,
    total_tiers        INT NOT NULL,
    acknowledged       BOOLEAN NOT NULL DEFAULT FALSE,
    acknowledged_by    TEXT,
    acknowledged_at    TIMESTAMPTZ,
    next_escalation_at TIMESTAMPTZ,
    status             TEXT NOT NULL DEFAULT 'active',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notify_escalation_states_pending
    ON escalation_states(status, next_escalation_at)
    WHERE status = 'active';
