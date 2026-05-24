import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { getAuth } from "./auth.server";

type BetterAuthSession = NonNullable<
  Awaited<ReturnType<Awaited<ReturnType<typeof getAuth>>["api"]["getSession"]>>
>;

type PublicSession = {
  user: BetterAuthSession["user"];
  session: Omit<BetterAuthSession["session"], "token">;
};

function toPublicSession(
  session: BetterAuthSession | null
): PublicSession | null {
  if (!session) {
    return null;
  }
  const { token: _token, ...publicSession } = session.session;
  return {
    user: session.user,
    session: publicSession,
  };
}

/**
 * Retrieves the current user session from request headers.
 * Returns null if no valid session exists.
 */
export const getSession = createServerFn({ method: "GET" }).handler(
  async () => {
    const headers = getRequestHeaders();
    const session = await (await getAuth()).api.getSession({ headers });
    return toPublicSession(session ?? null);
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

    return toPublicSession(session) as PublicSession;
  }
);
