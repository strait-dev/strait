-- Extend RLS coverage to every tenant-scoped table and FORCE row-level
-- security even for the table owner (which is currently the role the
-- application connects as). Without FORCE, the owner bypasses RLS and
-- the enforcement fix in the previous commit would not have any effect.
--
-- This migration lands on top of the per-request-transaction RLS
-- enforcement fix in the application layer: the rlsTxMiddleware begins
-- a tx, runs SELECT set_config('app.current_project_id', $1, true) on
-- it, and binds the tx to the request context. Every subsequent store
-- method call on that request runs on the same tx, so Postgres sees a
-- consistent current_setting('app.current_project_id') value for the
-- policy check. Combined with the FORCE here and the sentinel in the
-- next migration, RLS finally enforces tenant isolation end to end.

-- FORCE existing protected tables. These already had RLS enabled in
-- migration 000097 but never had FORCE, so the owner role (what the
-- app currently runs as) bypassed every policy.
ALTER TABLE jobs                  FORCE ROW LEVEL SECURITY;
ALTER TABLE job_runs              FORCE ROW LEVEL SECURITY;
ALTER TABLE workflows             FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_runs         FORCE ROW LEVEL SECURITY;
ALTER TABLE environments          FORCE ROW LEVEL SECURITY;
ALTER TABLE job_secrets           FORCE ROW LEVEL SECURITY;
ALTER TABLE api_keys              FORCE ROW LEVEL SECURITY;
ALTER TABLE webhook_subscriptions FORCE ROW LEVEL SECURITY;

-- Enable RLS + FORCE + tenant_isolation policy on the 13 tenant-scoped
-- tables that were missing it. Policy shape mirrors the existing pattern
-- from migration 000097 so the next migration (sentinel tightening) can
-- rewrite all policies in one place.

-- audit_events: actor/action/resource access trail
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_events
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- log_drains: outbound log delivery config including auth tokens in
-- auth_config JSONB, so cross-tenant leak here is credential exposure
ALTER TABLE log_drains ENABLE ROW LEVEL SECURITY;
ALTER TABLE log_drains FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON log_drains
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- notification_channels: encrypted Slack/Discord/webhook/email configs
ALTER TABLE notification_channels ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_channels FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notification_channels
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- notification_deliveries: delivered message payloads
ALTER TABLE notification_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_deliveries FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notification_deliveries
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- job_memory: per-project application state (JSONB values)
ALTER TABLE job_memory ENABLE ROW LEVEL SECURITY;
ALTER TABLE job_memory FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_memory
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- job_slos: SLO targets (business intelligence leak if cross-tenant)
ALTER TABLE job_slos ENABLE ROW LEVEL SECURITY;
ALTER TABLE job_slos FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_slos
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- job_slo_evaluations has no project_id column. Policy routes through
-- the parent job_slos. Any cross-tenant lookup that can't find the
-- parent in the current tenant returns zero rows.
ALTER TABLE job_slo_evaluations ENABLE ROW LEVEL SECURITY;
ALTER TABLE job_slo_evaluations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_slo_evaluations
    USING (EXISTS (
        SELECT 1 FROM job_slos s
        WHERE s.id = slo_id
          AND (s.project_id = current_setting('app.current_project_id', true)
               OR current_setting('app.current_project_id', true) = '')
    ));

-- event_triggers: request/response payloads + timing
ALTER TABLE event_triggers ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_triggers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON event_triggers
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- event_sources: external integration configuration
ALTER TABLE event_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_sources FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON event_sources
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- event_subscriptions has no project_id column. Policy routes through
-- the parent event_sources.
ALTER TABLE event_subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_subscriptions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON event_subscriptions
    USING (EXISTS (
        SELECT 1 FROM event_sources es
        WHERE es.id = source_id
          AND (es.project_id = current_setting('app.current_project_id', true)
               OR current_setting('app.current_project_id', true) = '')
    ));

-- workflow_versions: full workflow definitions
ALTER TABLE workflow_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workflow_versions
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- workflow_version_steps has no project_id column. Policy routes
-- through the parent workflow_versions.
ALTER TABLE workflow_version_steps ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_version_steps FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workflow_version_steps
    USING (EXISTS (
        SELECT 1 FROM workflow_versions wv
        WHERE wv.id = workflow_version_id
          AND (wv.project_id = current_setting('app.current_project_id', true)
               OR current_setting('app.current_project_id', true) = '')
    ));

-- cost_stats_hourly: SaaS unit economics
ALTER TABLE cost_stats_hourly ENABLE ROW LEVEL SECURITY;
ALTER TABLE cost_stats_hourly FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cost_stats_hourly
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- job_groups: categorization metadata
ALTER TABLE job_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE job_groups FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_groups
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- batch_operations: bulk operation tracking
ALTER TABLE batch_operations ENABLE ROW LEVEL SECURITY;
ALTER TABLE batch_operations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON batch_operations
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

-- workflow_policies: per-project governance rules
ALTER TABLE workflow_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_policies FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workflow_policies
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');
