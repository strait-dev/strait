import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { getAuth } from "./auth.server";

/**
 * Retrieves the current user session from request headers.
 * Returns null if no valid session exists.
 */
export const getSession = createServerFn({ method: "GET" }).handler(
  async () => {
    const headers = getRequestHeaders();
    const session = await (await getAuth()).api.getSession({ headers });
    return session ?? null;
  }
);

/**
 * Retrieves the current session or throws if not authenticated.
 * Use in server functions that require authentication.
 */
export const ensureSession = createServerFn({ method: "GET" }).handler(
  async () => {
    const headers = getRequestHeaders();
    const session = await (await getAuth()).api.getSession({ headers });

    if (!session) {
      throw new Error("Unauthorized");
    }

    return session;
  }
);
