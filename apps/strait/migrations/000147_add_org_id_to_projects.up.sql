ALTER TABLE projects ADD COLUMN IF NOT EXISTS org_id TEXT;
CREATE INDEX IF NOT EXISTS idx_projects_org_id ON projects(org_id);
