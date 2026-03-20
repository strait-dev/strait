/** Extract the active organization ID from a server function context session. */
export function getOrgIdFromSession(
  session: Record<string, unknown>
): string | null {
  const orgId = session.activeOrganizationId;
  if (!orgId || typeof orgId !== "string") {
    return null;
  }
  return orgId;
}
