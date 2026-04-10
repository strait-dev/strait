-- Add index on projects.org_id so ListCodeDeploymentsByOrg can join efficiently.
-- Without this index the JOIN projects p WHERE p.org_id = $1 does a full table scan
-- on every admin deployment listing request.
CREATE INDEX IF NOT EXISTS idx_projects_org_id ON projects (org_id);
