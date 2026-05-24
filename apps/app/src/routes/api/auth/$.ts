import { createFileRoute } from "@tanstack/react-router";
import { getAuth, getAuthPool } from "@/lib/auth.server";
import { handler } from "@/lib/auth-handler.server";

const MEMBER_ROLES = new Set(["owner", "admin", "member"]);

async function guardMemberRoleUpdate(
  request: Request
): Promise<Response | null> {
  const url = new URL(request.url);
  if (!url.pathname.endsWith("/api/auth/organization/update-member-role")) {
    return null;
  }

  let body: unknown;
  try {
    body = await request.clone().json();
  } catch {
    return Response.json({ error: "Invalid request" }, { status: 400 });
  }

  const input = body as {
    memberId?: unknown;
    organizationId?: unknown;
    role?: unknown;
  };
  if (
    typeof input.memberId !== "string" ||
    typeof input.organizationId !== "string" ||
    typeof input.role !== "string" ||
    !MEMBER_ROLES.has(input.role)
  ) {
    return Response.json({ error: "Invalid member role" }, { status: 400 });
  }

  const session = await (await getAuth()).api.getSession({
    headers: request.headers,
  });
  if (!session?.user?.id) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const pool = getAuthPool();
  const target = await pool.query<{ userId: string }>(
    'SELECT "userId" FROM member WHERE id = $1 AND "organizationId" = $2',
    [input.memberId, input.organizationId]
  );
  if (!target.rows[0]) {
    return Response.json({ error: "Member not found" }, { status: 404 });
  }
  if (target.rows[0].userId === session.user.id) {
    return Response.json(
      { error: "Owners cannot change their own role" },
      { status: 403 }
    );
  }

  const caller = await pool.query<{ role: string }>(
    'SELECT "role" FROM member WHERE "userId" = $1 AND "organizationId" = $2 LIMIT 1',
    [session.user.id, input.organizationId]
  );
  if (caller.rows[0]?.role !== "owner") {
    return Response.json({ error: "Forbidden" }, { status: 403 });
  }

  return null;
}

async function handlePost(request: Request): Promise<Response> {
  const guard = await guardMemberRoleUpdate(request);
  if (guard) {
    return guard;
  }
  return handler(request);
}

/**
 * Better Auth route handler for authentication endpoints.
 * Proxies all /api/auth/* routes to the Better Auth handler.
 */
export const Route = createFileRoute("/api/auth/$")({
  server: {
    handlers: {
      GET: ({ request }) => handler(request),
      POST: ({ request }) => handlePost(request),
    },
  },
});
