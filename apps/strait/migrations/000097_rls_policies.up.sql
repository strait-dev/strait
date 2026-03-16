-- Enable Row Level Security on tenant-scoped tables.
-- Table owner bypasses RLS by default (no FORCE), so this is safe for system operations.

ALTER TABLE jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE job_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflows ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE environments ENABLE ROW LEVEL SECURITY;
ALTER TABLE job_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_subscriptions ENABLE ROW LEVEL SECURITY;

-- Tenant isolation policies: restrict rows to the current project context.
CREATE POLICY tenant_isolation ON jobs
    USING (project_id = current_setting('app.current_project_id', true) OR current_setting('app.current_project_id', true) = '');

CREATE POLICY tenant_isolation ON job_runs
    USING (project_id = current_setting('app.current_project_id', true) OR current_setting('app.current_project_id', true) = '');

CREATE POLICY tenant_isolation ON workflows
    USING (project_id = current_setting('app.current_project_id', true) OR current_setting('app.current_project_id', true) = '');

CREATE POLICY tenant_isolation ON workflow_runs
    USING (project_id = current_setting('app.current_project_id', true) OR current_setting('app.current_project_id', true) = '');

CREATE POLICY tenant_isolation ON environments
    USING (project_id = current_setting('app.current_project_id', true) OR current_setting('app.current_project_id', true) = '');

CREATE POLICY tenant_isolation ON job_secrets
    USING (project_id = current_setting('app.current_project_id', true) OR current_setting('app.current_project_id', true) = '');

CREATE POLICY tenant_isolation ON api_keys
    USING (project_id = current_setting('app.current_project_id', true) OR current_setting('app.current_project_id', true) = '');

CREATE POLICY tenant_isolation ON webhook_subscriptions
    USING (project_id = current_setting('app.current_project_id', true) OR current_setting('app.current_project_id', true) = '');
