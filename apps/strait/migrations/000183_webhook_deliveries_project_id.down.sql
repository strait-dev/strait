DROP POLICY IF EXISTS tenant_isolation ON webhook_deliveries;
ALTER TABLE webhook_deliveries NO FORCE ROW LEVEL SECURITY;
ALTER TABLE webhook_deliveries DISABLE ROW LEVEL SECURITY;
DROP INDEX IF EXISTS idx_webhook_deliveries_project_created;
ALTER TABLE webhook_deliveries DROP COLUMN IF EXISTS project_id;
