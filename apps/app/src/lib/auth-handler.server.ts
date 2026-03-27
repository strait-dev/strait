import { OAUTH_API_CORS_HEADERS } from "@/lib/oauth-scopes";
import { auth } from "./auth.server";

const OAUTH_CORS_PATHS = [
  "/api/auth/oauth2/token",
  "/api/auth/oauth2/register",
  "/api/auth/oauth2/revoke",
  "/api/auth/oauth2/userinfo",
  "/api/auth/jwks",
];

function needsCors(request: Request): boolean {
  const url = new URL(request.url);
  return OAUTH_CORS_PATHS.some((p) => url.pathname === p);
}

export const handler = async (request: Request): Promise<Response> => {
  if (request.method === "OPTIONS" && needsCors(request)) {
    return new Response(null, { status: 204, headers: OAUTH_API_CORS_HEADERS });
  }

  const response = await auth.handler(request);

  if (needsCors(request)) {
    const headers = new Headers(response.headers);
    for (const [key, value] of Object.entries(OAUTH_API_CORS_HEADERS)) {
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
