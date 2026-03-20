CREATE TABLE IF NOT EXISTS usage_records (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    org_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    period_date DATE NOT NULL,
    runs_count BIGINT NOT NULL DEFAULT 0,
    compute_cost_microusd BIGINT NOT NULL DEFAULT 0,
    ai_tokens_total BIGINT NOT NULL DEFAULT 0,
    ai_cost_microusd BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, project_id, period_date)
);

CREATE INDEX IF NOT EXISTS idx_usage_records_org_period ON usage_records(org_id, period_date);
CREATE INDEX IF NOT EXISTS idx_usage_records_project_period ON usage_records(project_id, period_date);
