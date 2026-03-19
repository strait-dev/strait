-- Write your UP migration here
ALTER TABLE projects
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_projects_org_id_active
ON projects (org_id)
WHERE deleted_at IS NULL;
