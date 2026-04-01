import { getAuthPool } from "@/lib/auth.server";

/**
 * Validates that the user is a member of the given organization.
 * Throws "Forbidden" if not.
 */
export async function requireOrgAccess(
  userId: string,
  organizationId: string
): Promise<void> {
  if (!(userId && organizationId)) {
    throw new Error("Forbidden");
  }

  const result = await getAuthPool().query(
    'SELECT 1 FROM member WHERE "userId" = $1 AND "organizationId" = $2',
    [userId, organizationId]
  );

  if (result.rowCount === 0) {
    throw new Error("Forbidden");
  }
}

/**
 * Validates that the user owns the project via org membership.
 * 1. Checks user is a member of activeOrganizationId
 * 2. Checks project belongs to that org
 * Throws "Forbidden" if either check fails.
 */
export async function requireProjectAccess(
  userId: string,
  projectId: string,
  activeOrganizationId: string | undefined
): Promise<void> {
  if (!(userId && projectId && activeOrganizationId)) {
    throw new Error("Forbidden");
  }

  await requireOrgAccess(userId, activeOrganizationId);

  const result = await getAuthPool().query(
    "SELECT 1 FROM project WHERE id = $1 AND organization_id = $2",
    [projectId, activeOrganizationId]
  );

  if (result.rowCount === 0) {
    throw new Error("Forbidden");
  }
}
