import { getAuthPool } from "@/lib/auth.server";

export type OrganizationRole = "owner" | "admin" | "member";

export type AuthzContext = {
  user: { id: string };
  activeOrganizationId?: string;
  activeProjectId?: string;
};

const roleRank: Record<OrganizationRole, number> = {
  member: 1,
  admin: 2,
  owner: 3,
};

function parseStoredRoles(role: unknown): OrganizationRole[] {
  if (typeof role !== "string") {
    return [];
  }

  const normalized = role.trim();
  if (
    normalized === "owner" ||
    normalized === "admin" ||
    normalized === "member"
  ) {
    return [normalized];
  }
  return [];
}

function hasRequiredRole(
  storedRole: unknown,
  minimumRole: OrganizationRole
): boolean {
  return parseStoredRoles(storedRole).some(
    (role) => roleRank[role] >= roleRank[minimumRole]
  );
}

export async function getOrganizationRole(
  userId: string,
  organizationId: string
): Promise<OrganizationRole | null> {
  if (!(userId && organizationId)) {
    throw new Error("Forbidden");
  }

  const result = await getAuthPool().query<{ role: string }>(
    'SELECT "role" FROM member WHERE "userId" = $1 AND "organizationId" = $2 LIMIT 1',
    [userId, organizationId]
  );

  const roles = parseStoredRoles(result.rows[0]?.role);
  if (roles.includes("owner")) {
    return "owner";
  }
  if (roles.includes("admin")) {
    return "admin";
  }
  if (roles.includes("member")) {
    return "member";
  }
  return null;
}

export async function getProjectOrganizationRole(
  userId: string,
  projectId: string
): Promise<OrganizationRole | null> {
  if (!(userId && projectId)) {
    throw new Error("Forbidden");
  }

  const result = await getAuthPool().query<{ role: string }>(
    `SELECT m."role"
     FROM project p
     JOIN member m ON m."organizationId" = p.organization_id
     WHERE p.id = $1 AND m."userId" = $2
     LIMIT 1`,
    [projectId, userId]
  );

  const roles = parseStoredRoles(result.rows[0]?.role);
  if (roles.includes("owner")) {
    return "owner";
  }
  if (roles.includes("admin")) {
    return "admin";
  }
  if (roles.includes("member")) {
    return "member";
  }
  return null;
}

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

export async function requireOrgRole(
  userId: string,
  organizationId: string,
  minimumRole: OrganizationRole
): Promise<void> {
  if (!(userId && organizationId)) {
    throw new Error("Forbidden");
  }

  const result = await getAuthPool().query<{ role: string }>(
    'SELECT "role" FROM member WHERE "userId" = $1 AND "organizationId" = $2 LIMIT 1',
    [userId, organizationId]
  );

  if (!hasRequiredRole(result.rows[0]?.role, minimumRole)) {
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

export async function requireProjectRole(
  userId: string,
  projectId: string,
  activeOrganizationId: string | undefined,
  minimumRole: OrganizationRole
): Promise<void> {
  await requireProjectAccess(userId, projectId, activeOrganizationId);
  await requireOrgRole(userId, activeOrganizationId ?? "", minimumRole);
}

export async function requireOrgAdmin(
  userId: string,
  organizationId: string
): Promise<void> {
  await requireOrgRole(userId, organizationId, "admin");
}

export async function requireOrgOwner(
  userId: string,
  organizationId: string
): Promise<void> {
  await requireOrgRole(userId, organizationId, "owner");
}

export async function requireProjectAdmin(
  userId: string,
  projectId: string,
  activeOrganizationId: string | undefined
): Promise<void> {
  await requireProjectRole(userId, projectId, activeOrganizationId, "admin");
}

export async function requireActiveProjectAccess(
  context: AuthzContext
): Promise<string> {
  const projectId = context.activeProjectId;
  if (!projectId) {
    throw new Error("Forbidden");
  }
  await requireProjectAccess(
    context.user.id,
    projectId,
    context.activeOrganizationId
  );
  return projectId as string;
}

export async function requireActiveOrgAccess(
  context: AuthzContext
): Promise<string> {
  const orgId = context.activeOrganizationId;
  if (!orgId) {
    throw new Error("Forbidden");
  }
  await requireOrgAccess(context.user.id, orgId);
  return orgId;
}

export async function requireActiveOrgAdmin(
  context: AuthzContext
): Promise<string> {
  const orgId = context.activeOrganizationId;
  if (!orgId) {
    throw new Error("Forbidden");
  }
  await requireOrgAdmin(context.user.id, orgId);
  return orgId;
}

export async function requireActiveProjectAdmin(
  context: AuthzContext
): Promise<string> {
  const projectId = context.activeProjectId;
  if (!projectId) {
    throw new Error("Forbidden");
  }
  await requireProjectAdmin(
    context.user.id,
    projectId,
    context.activeOrganizationId
  );
  return projectId as string;
}
