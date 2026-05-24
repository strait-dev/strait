CREATE TABLE IF NOT EXISTS billing_cost_events (
    idempotency_key TEXT PRIMARY KEY,
    org_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    period_date DATE NOT NULL,
    execution_mode TEXT NOT NULL,
    compute_cost_microusd BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_billing_cost_events_org_period
    ON billing_cost_events(org_id, period_date);

CREATE INDEX IF NOT EXISTS idx_billing_cost_events_project_period
    ON billing_cost_events(project_id, period_date);

INSERT INTO billing_cost_events (
    idempotency_key, org_id, project_id, period_date, execution_mode,
    compute_cost_microusd, created_at
)
SELECT
    'strait:cost_recorded:' || jr.id,
    p.org_id,
    jr.project_id,
    DATE(COALESCE(jr.finished_at, jr.created_at)),
    COALESCE(NULLIF(jr.execution_mode, ''), 'http'),
    CASE WHEN COALESCE(NULLIF(jr.execution_mode, ''), 'http') = 'worker' THEN 20 ELSE 20 END,
    COALESCE(jr.finished_at, jr.created_at)
FROM job_runs jr
JOIN projects p ON p.id = jr.project_id
WHERE jr.status = 'completed'
  AND p.org_id IS NOT NULL
ON CONFLICT (idempotency_key) DO NOTHING;

INSERT INTO billing_cost_events (
    idempotency_key, org_id, project_id, period_date, execution_mode,
    compute_cost_microusd, created_at
)
SELECT
    'strait:cost_recorded:' || wd.id,
    p.org_id,
    wd.project_id,
    DATE(COALESCE(wd.delivered_at, wd.updated_at, wd.created_at)),
    'webhook_delivery',
    20,
    COALESCE(wd.delivered_at, wd.updated_at, wd.created_at)
FROM webhook_deliveries wd
JOIN projects p ON p.id = wd.project_id
WHERE wd.status = 'delivered'
  AND p.org_id IS NOT NULL
ON CONFLICT (idempotency_key) DO NOTHING;
