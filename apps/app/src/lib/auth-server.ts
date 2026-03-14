import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";

/**
 * Better Auth handler for the API route.
 * Processes all /api/auth/* requests.
 */
export const handler = async (request: Request) => {
  const { auth } = await import("./auth");
  return auth.handler(request);
};

/**
 * Retrieves the current user session from request headers.
 * Returns null if no valid session exists.
 */
export const getSession = createServerFn({ method: "GET" }).handler(
  async () => {
    const { auth } = await import("./auth");
    const headers = getRequestHeaders();
    const session = await auth.api.getSession({ headers });
    return session ?? null;
  }
);

/**
 * Retrieves the current session or throws if not authenticated.
 * Use in server functions that require authentication.
 */
export const ensureSession = createServerFn({ method: "GET" }).handler(
  async () => {
    const { auth } = await import("./auth");
    const headers = getRequestHeaders();
    const session = await auth.api.getSession({ headers });

    if (!session) {
      throw new Error("Unauthorized");
    }

    return session;
  }
);
