/**
 * Session utilities for billing server functions.
 *
 * Extracts the active organization ID from server function context,
 * used by all billing hooks to scope API calls to the correct org.
 */

/**
 * Extract the active organization ID from a server function context session.
 *
 * @param session - The raw session object from the server function context.
 * @returns The active organization ID string, or `null` if not present or invalid.
 */
export const getOrgIdFromSession = (
  session: Record<string, unknown>
): string | null => {
  const orgId = session.activeOrganizationId;
  if (!orgId || typeof orgId !== "string") {
    return null;
  }
  return orgId;
};
