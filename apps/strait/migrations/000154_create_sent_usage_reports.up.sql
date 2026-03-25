CREATE TABLE IF NOT EXISTS sent_usage_reports (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    org_id TEXT NOT NULL,
    period_end DATE NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, period_end)
);

CREATE INDEX IF NOT EXISTS idx_sent_usage_reports_org_id ON sent_usage_reports(org_id);
