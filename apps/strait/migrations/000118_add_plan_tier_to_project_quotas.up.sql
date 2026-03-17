-- Add plan_tier to project_quotas for plan-based region gating.
ALTER TABLE project_quotas ADD COLUMN IF NOT EXISTS plan_tier TEXT DEFAULT 'free';
