-- Backfill org_id for existing projects that have NULL org_id.
-- For each project without an org_id, we look up the user who created it
-- (via the first run or the project_member_roles table) and assign the
-- project to that user's first known org from organization_subscriptions.
-- This is idempotent — only affects rows where org_id IS NULL.

DO $$
BEGIN
  -- Only run if there are projects with NULL org_id
  IF EXISTS (SELECT 1 FROM projects WHERE org_id IS NULL) THEN
    -- For projects created by a user who has an org subscription,
    -- assign the project to that org
    UPDATE projects p
    SET org_id = sub.org_id
    FROM (
      SELECT DISTINCT ON (pmr.project_id) pmr.project_id, os.org_id
      FROM project_member_roles pmr
      JOIN organization_subscriptions os ON os.org_id IS NOT NULL
      WHERE pmr.project_id IN (SELECT id FROM projects WHERE org_id IS NULL)
      ORDER BY pmr.project_id, os.created_at ASC
    ) sub
    WHERE p.id = sub.project_id AND p.org_id IS NULL;
  END IF;
END $$;
