ALTER TABLE project_quotas
  ADD COLUMN IF NOT EXISTS monthly_budget_microusd BIGINT DEFAULT -1,
  ADD COLUMN IF NOT EXISTS budget_action TEXT DEFAULT 'notify';
