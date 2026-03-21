ALTER TABLE project_quotas
  DROP COLUMN IF EXISTS monthly_budget_microusd,
  DROP COLUMN IF EXISTS budget_action;
