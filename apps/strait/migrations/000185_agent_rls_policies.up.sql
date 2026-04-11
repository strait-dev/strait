-- Tenant-isolation RLS policies for every agent table and the Phase
-- C/D tenant-scoped platform tables.
--
-- Context: migration 000182 extended RLS coverage to Jobs, Workflows,
-- Environments, Events, Notifications, etc. — but every `agents`,
-- `agent_*`, and `project_secrets` / `project_*_quotas` /
-- `project_platform_settings` table was left unprotected. Cross-tenant
-- reads on those tables are enforced today only by application-layer
-- guards, which is fine in practice but leaves no defense in depth.
--
-- Policy shape mirrors migration 000182:
--
--   USING (project_id = current_setting('app.current_project_id', true)
--          OR current_setting('app.current_project_id', true) = '')
--
-- The empty-string fallback keeps unscoped admin queries (internal
-- tooling, webhook handlers, background workers) working — they enter
-- the request without a project context and see every row. Request-
-- scoped traffic goes through the rlsTxMiddleware which sets
-- app.current_project_id at the start of every tx.
--
-- Tables without a project_id column (agent_deployments) route through
-- their parent via an EXISTS subquery, matching the job_slo_evaluations
-- / event_subscriptions pattern from 000182.

-- agents: direct project_id scoping
ALTER TABLE agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE agents FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agents
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- agent_deployments has no project_id column. Policy routes through
-- the parent agents row: a deployment is visible iff its agent is
-- visible under the current tenant context.
ALTER TABLE agent_deployments ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_deployments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agent_deployments
    USING (EXISTS (
        SELECT 1 FROM agents a
        WHERE a.id = agent_id
          AND (a.project_id = current_setting('app.current_project_id', true)
               OR current_setting('app.current_project_id', true) = '')
    ));

-- agent_canary_deployments: direct project_id scoping
ALTER TABLE agent_canary_deployments ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_canary_deployments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agent_canary_deployments
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- agent_messages: direct project_id scoping. Important because this
-- table carries arbitrary JSONB payloads passed between agents — a
-- cross-tenant leak is a data exposure.
ALTER TABLE agent_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_messages FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agent_messages
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- agent_usage_records: direct project_id scoping. Contains billing
-- aggregates (token counts, costs) per project.
ALTER TABLE agent_usage_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_usage_records FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON agent_usage_records
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- project_secrets: direct project_id scoping. This is the Phase D
-- platform primitive — env-scoped secrets shared by Jobs and Agents.
-- Cross-tenant leak is a credential exposure.
ALTER TABLE project_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_secrets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON project_secrets
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- project_job_quotas, project_agent_quotas, project_platform_settings:
-- the Phase C split tables. Plan limits + billing settings. Low leak
-- severity but worth the defense in depth.
ALTER TABLE project_job_quotas ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_job_quotas FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON project_job_quotas
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

ALTER TABLE project_agent_quotas ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_agent_quotas FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON project_agent_quotas
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

ALTER TABLE project_platform_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_platform_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON project_platform_settings
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');
