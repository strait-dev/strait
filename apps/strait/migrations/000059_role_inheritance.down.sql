DROP INDEX IF EXISTS idx_project_roles_parent_role_id;
ALTER TABLE project_roles DROP COLUMN IF EXISTS parent_role_id;
