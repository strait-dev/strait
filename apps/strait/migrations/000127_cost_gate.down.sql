DROP TABLE IF EXISTS job_cost_estimates;

ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS cost_gate_default_action;
ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS cost_gate_timeout_secs;
ALTER TABLE workflow_version_steps DROP COLUMN IF EXISTS cost_gate_threshold_microusd;

ALTER TABLE workflow_steps DROP COLUMN IF EXISTS cost_gate_default_action;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS cost_gate_timeout_secs;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS cost_gate_threshold_microusd;
