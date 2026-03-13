ALTER TABLE project_roles
    ADD COLUMN IF NOT EXISTS parent_role_id TEXT REFERENCES project_roles(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_project_roles_parent_role_id ON project_roles(parent_role_id);
