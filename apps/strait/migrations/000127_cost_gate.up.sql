ALTER TABLE workflow_steps ADD COLUMN cost_gate_threshold_microusd BIGINT;
ALTER TABLE workflow_steps ADD COLUMN cost_gate_timeout_secs INT;
ALTER TABLE workflow_steps ADD COLUMN cost_gate_default_action TEXT;

ALTER TABLE workflow_version_steps ADD COLUMN cost_gate_threshold_microusd BIGINT;
ALTER TABLE workflow_version_steps ADD COLUMN cost_gate_timeout_secs INT;
ALTER TABLE workflow_version_steps ADD COLUMN cost_gate_default_action TEXT;

CREATE TABLE job_cost_estimates (
    job_id            TEXT PRIMARY KEY,
    avg_cost_microusd BIGINT NOT NULL DEFAULT 0,
    sample_count      INT NOT NULL DEFAULT 0,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
