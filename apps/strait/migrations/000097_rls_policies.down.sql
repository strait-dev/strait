DROP POLICY IF EXISTS tenant_isolation ON webhook_subscriptions;
DROP POLICY IF EXISTS tenant_isolation ON api_keys;
DROP POLICY IF EXISTS tenant_isolation ON job_secrets;
DROP POLICY IF EXISTS tenant_isolation ON environments;
DROP POLICY IF EXISTS tenant_isolation ON workflow_runs;
DROP POLICY IF EXISTS tenant_isolation ON workflows;
DROP POLICY IF EXISTS tenant_isolation ON job_runs;
DROP POLICY IF EXISTS tenant_isolation ON jobs;

ALTER TABLE webhook_subscriptions DISABLE ROW LEVEL SECURITY;
ALTER TABLE api_keys DISABLE ROW LEVEL SECURITY;
ALTER TABLE job_secrets DISABLE ROW LEVEL SECURITY;
ALTER TABLE environments DISABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_runs DISABLE ROW LEVEL SECURITY;
ALTER TABLE workflows DISABLE ROW LEVEL SECURITY;
ALTER TABLE job_runs DISABLE ROW LEVEL SECURITY;
ALTER TABLE jobs DISABLE ROW LEVEL SECURITY;
