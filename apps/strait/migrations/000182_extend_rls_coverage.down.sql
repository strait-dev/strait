-- Drop the tenant_isolation policies added in the up migration and
-- disable RLS on the newly-protected tables. Do not disable RLS on
-- the tables that were already protected by migration 000097, just
-- drop the FORCE bit.

DROP POLICY IF EXISTS tenant_isolation ON workflow_policies;
ALTER TABLE workflow_policies NO FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_policies DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON batch_operations;
ALTER TABLE batch_operations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE batch_operations DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON job_groups;
ALTER TABLE job_groups NO FORCE ROW LEVEL SECURITY;
ALTER TABLE job_groups DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON cost_stats_hourly;
ALTER TABLE cost_stats_hourly NO FORCE ROW LEVEL SECURITY;
ALTER TABLE cost_stats_hourly DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON workflow_version_steps;
ALTER TABLE workflow_version_steps NO FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_version_steps DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON workflow_versions;
ALTER TABLE workflow_versions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_versions DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON event_subscriptions;
ALTER TABLE event_subscriptions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE event_subscriptions DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON event_sources;
ALTER TABLE event_sources NO FORCE ROW LEVEL SECURITY;
ALTER TABLE event_sources DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON event_triggers;
ALTER TABLE event_triggers NO FORCE ROW LEVEL SECURITY;
ALTER TABLE event_triggers DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON job_slo_evaluations;
ALTER TABLE job_slo_evaluations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE job_slo_evaluations DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON job_slos;
ALTER TABLE job_slos NO FORCE ROW LEVEL SECURITY;
ALTER TABLE job_slos DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON job_memory;
ALTER TABLE job_memory NO FORCE ROW LEVEL SECURITY;
ALTER TABLE job_memory DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON notification_deliveries;
ALTER TABLE notification_deliveries NO FORCE ROW LEVEL SECURITY;
ALTER TABLE notification_deliveries DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON notification_channels;
ALTER TABLE notification_channels NO FORCE ROW LEVEL SECURITY;
ALTER TABLE notification_channels DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON log_drains;
ALTER TABLE log_drains NO FORCE ROW LEVEL SECURITY;
ALTER TABLE log_drains DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON audit_events;
ALTER TABLE audit_events NO FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_events DISABLE ROW LEVEL SECURITY;

-- Remove FORCE from the 000097 tables (RLS stays enabled, they just
-- go back to owner-bypass).
ALTER TABLE webhook_subscriptions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE api_keys              NO FORCE ROW LEVEL SECURITY;
ALTER TABLE job_secrets           NO FORCE ROW LEVEL SECURITY;
ALTER TABLE environments          NO FORCE ROW LEVEL SECURITY;
ALTER TABLE workflow_runs         NO FORCE ROW LEVEL SECURITY;
ALTER TABLE workflows             NO FORCE ROW LEVEL SECURITY;
ALTER TABLE job_runs              NO FORCE ROW LEVEL SECURITY;
ALTER TABLE jobs                  NO FORCE ROW LEVEL SECURITY;
