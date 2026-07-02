import { createMiddleware } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { getAuth } from "@/lib/auth.server";

/**
 * Auth middleware for server functions.
 * Validates the session and attaches user context.
 * Throws if the user is not authenticated.
 */
export const authMiddleware = createMiddleware().server(async ({ next }) => {
  const headers = getRequestHeaders();
  const session = await (await getAuth()).api.getSession({ headers });

  if (!session) {
    throw new Error("Unauthorized");
  }

  return next({
    context: {
      user: {
        id: session.user.id,
        name: session.user.name,
        email: session.user.email,
        createdAt: session.user.createdAt,
        updatedAt: session.user.updatedAt,
        defaultOrganizationId: (session.user as Record<string, unknown>)
          .defaultOrganizationId as string | undefined,
        activeProjectId: (session.user as Record<string, unknown>)
          .activeProjectId as string | undefined,
      },
      session: session.session,
      activeOrganizationId:
        ((session.session as Record<string, unknown>).activeOrganizationId as
          | string
          | undefined) ??
        ((session.user as Record<string, unknown>).defaultOrganizationId as
          | string
          | undefined),
      activeProjectId: (session.user as Record<string, unknown>)
        .activeProjectId as string | undefined,
    },
  });
});
