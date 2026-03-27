import { auth } from "./auth.server";

const OAUTH_CORS_PATHS = [
  "/api/auth/oauth2/token",
  "/api/auth/oauth2/register",
  "/api/auth/oauth2/revoke",
  "/api/auth/oauth2/userinfo",
  "/api/auth/jwks",
];

const CORS_HEADERS: Record<string, string> = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
  "Access-Control-Allow-Headers": "Content-Type, Authorization",
  "Access-Control-Max-Age": "86400",
};

function needsCors(request: Request): boolean {
  const url = new URL(request.url);
  return OAUTH_CORS_PATHS.some((p) => url.pathname === p);
}

/**
 * Better Auth handler for the API route.
 * Processes all /api/auth/* requests.
 * Adds CORS headers to OAuth endpoints for browser-based MCP clients.
 */
export const handler = async (request: Request): Promise<Response> => {
  // Handle CORS preflight for OAuth endpoints.
  if (request.method === "OPTIONS" && needsCors(request)) {
    return new Response(null, { status: 204, headers: CORS_HEADERS });
  }

  const response = await auth.handler(request);

  // Append CORS headers to OAuth endpoint responses.
  if (needsCors(request)) {
    const headers = new Headers(response.headers);
    for (const [key, value] of Object.entries(CORS_HEADERS)) {
      headers.set(key, value);
    }
    return new Response(response.body, {
      status: response.status,
      statusText: response.statusText,
      headers,
    });
  }

  return response;
};
