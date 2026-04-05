CREATE TABLE IF NOT EXISTS agent_usage_records (
    id TEXT PRIMARY KEY DEFAULT gen_random_ulid(),
    run_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    total_tokens BIGINT NOT NULL DEFAULT 0,
    tool_call_count INT NOT NULL DEFAULT 0,
    run_cost_microusd BIGINT NOT NULL DEFAULT 1000,
    token_cost_microusd BIGINT NOT NULL DEFAULT 0,
    tool_cost_microusd BIGINT NOT NULL DEFAULT 0,
    total_cost_microusd BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_usage_records_org_created
    ON agent_usage_records (org_id, created_at);
CREATE INDEX idx_agent_usage_records_project_created
    ON agent_usage_records (project_id, created_at);
CREATE UNIQUE INDEX idx_agent_usage_records_run_id
    ON agent_usage_records (run_id);
