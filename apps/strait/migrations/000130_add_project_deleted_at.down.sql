-- Write your DOWN migration here
DROP INDEX IF EXISTS idx_projects_org_id_active;

ALTER TABLE projects
DROP COLUMN IF EXISTS deleted_at;
