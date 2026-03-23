DROP INDEX IF EXISTS idx_projects_org_id;
ALTER TABLE projects DROP COLUMN IF EXISTS org_id;
