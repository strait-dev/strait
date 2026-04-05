CREATE TABLE IF NOT EXISTS eval_runs (
    id TEXT PRIMARY KEY DEFAULT gen_random_ulid(),
    agent_id TEXT NOT NULL,
    deployment_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    suite_name TEXT NOT NULL,
    results_json JSONB NOT NULL DEFAULT '{}',
    total_cases INT NOT NULL DEFAULT 0,
    passed_cases INT NOT NULL DEFAULT 0,
    failed_cases INT NOT NULL DEFAULT 0,
    duration_ms INT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'completed',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_eval_runs_agent ON eval_runs (agent_id, created_at DESC);
CREATE INDEX idx_eval_runs_project ON eval_runs (project_id, created_at DESC);
