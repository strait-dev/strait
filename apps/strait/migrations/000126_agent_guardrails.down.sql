DROP INDEX IF EXISTS idx_run_iterations_run_id;
DROP TABLE IF EXISTS run_iterations;

ALTER TABLE jobs DROP COLUMN IF EXISTS blocked_tools;
ALTER TABLE jobs DROP COLUMN IF EXISTS allowed_tools;
ALTER TABLE jobs DROP COLUMN IF EXISTS max_iterations_per_run;
ALTER TABLE jobs DROP COLUMN IF EXISTS max_tool_calls_per_run;
ALTER TABLE jobs DROP COLUMN IF EXISTS max_tokens_per_run;

ALTER TABLE project_quotas DROP COLUMN IF EXISTS max_iterations_per_run;
ALTER TABLE project_quotas DROP COLUMN IF EXISTS max_tool_calls_per_run;
ALTER TABLE project_quotas DROP COLUMN IF EXISTS max_tokens_per_run;
