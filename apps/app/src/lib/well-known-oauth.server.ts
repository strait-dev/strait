import { getAuth } from "@/lib/auth.server";
import { OAUTH_CORS_HEADERS } from "@/lib/oauth-scopes";

const WELL_KNOWN_PATHS = new Set([
  "/.well-known/oauth-authorization-server",
  "/.well-known/openid-configuration",
]);

export function isWellKnownOAuthRequest(request: Request): boolean {
  return WELL_KNOWN_PATHS.has(new URL(request.url).pathname);
}

export async function handleWellKnownOAuthRequest(
  request: Request
): Promise<Response | null> {
  const url = new URL(request.url);
  if (!WELL_KNOWN_PATHS.has(url.pathname)) {
    return null;
  }

  if (request.method === "OPTIONS") {
    return new Response(null, { status: 204, headers: OAUTH_CORS_HEADERS });
  }

  if (request.method !== "GET" && request.method !== "HEAD") {
    return new Response(JSON.stringify({ error: "method_not_allowed" }), {
      status: 405,
      headers: {
        "Content-Type": "application/json",
        Allow: "GET, HEAD, OPTIONS",
        ...OAUTH_CORS_HEADERS,
      },
    });
  }

  const auth = await getAuth();
  const api = auth.api as {
    getOAuthServerConfig?: () => Promise<unknown>;
    getOpenIdConfig?: () => Promise<unknown>;
  };
  const data =
    url.pathname === "/.well-known/oauth-authorization-server"
      ? await api.getOAuthServerConfig?.()
      : await api.getOpenIdConfig?.();

  if (!data) {
    return new Response(JSON.stringify({ error: "not_found" }), {
      status: 404,
      headers: { "Content-Type": "application/json", ...OAUTH_CORS_HEADERS },
    });
  }

  return new Response(request.method === "HEAD" ? null : JSON.stringify(data), {
    headers: {
      "Content-Type": "application/json",
      ...OAUTH_CORS_HEADERS,
    },
  });
}
