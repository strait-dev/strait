ALTER TABLE project_quotas ADD COLUMN max_tokens_per_run BIGINT;
ALTER TABLE project_quotas ADD COLUMN max_tool_calls_per_run INT;
ALTER TABLE project_quotas ADD COLUMN max_iterations_per_run INT;

ALTER TABLE jobs ADD COLUMN max_tokens_per_run BIGINT;
ALTER TABLE jobs ADD COLUMN max_tool_calls_per_run INT;
ALTER TABLE jobs ADD COLUMN max_iterations_per_run INT;
ALTER TABLE jobs ADD COLUMN allowed_tools TEXT[];
ALTER TABLE jobs ADD COLUMN blocked_tools TEXT[];

CREATE TABLE run_iterations (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    run_id      TEXT NOT NULL,
    iteration   INT NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_run_iterations_run_id ON run_iterations(run_id);
