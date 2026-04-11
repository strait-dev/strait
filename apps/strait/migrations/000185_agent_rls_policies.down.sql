-- Roll back the Phase E4 tenant-isolation policies. Drops policies
-- first, then FORCE ROW LEVEL SECURITY, then ENABLE ROW LEVEL SECURITY.

DROP POLICY IF EXISTS tenant_isolation ON project_platform_settings;
ALTER TABLE project_platform_settings NO FORCE ROW LEVEL SECURITY;
ALTER TABLE project_platform_settings DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON project_agent_quotas;
ALTER TABLE project_agent_quotas NO FORCE ROW LEVEL SECURITY;
ALTER TABLE project_agent_quotas DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON project_job_quotas;
ALTER TABLE project_job_quotas NO FORCE ROW LEVEL SECURITY;
ALTER TABLE project_job_quotas DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON project_secrets;
ALTER TABLE project_secrets NO FORCE ROW LEVEL SECURITY;
ALTER TABLE project_secrets DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON agent_usage_records;
ALTER TABLE agent_usage_records NO FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_usage_records DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON agent_messages;
ALTER TABLE agent_messages NO FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_messages DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON agent_canary_deployments;
ALTER TABLE agent_canary_deployments NO FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_canary_deployments DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON agent_deployments;
ALTER TABLE agent_deployments NO FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_deployments DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON agents;
ALTER TABLE agents NO FORCE ROW LEVEL SECURITY;
ALTER TABLE agents DISABLE ROW LEVEL SECURITY;
