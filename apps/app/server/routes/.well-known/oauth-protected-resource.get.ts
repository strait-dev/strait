import { defineEventHandler } from "vinxi/http";

export default defineEventHandler(async (event) => {
  try {
    const { OAUTH_CORS_HEADERS, ALL_OAUTH_SCOPES } = await import(
      "../../../src/lib/oauth-scopes"
    );

    const straitApiUrl = process.env.STRAIT_API_URL || "http://localhost:8080";
    const oidcIssuer =
      process.env.OIDC_ISSUER || process.env.BETTER_AUTH_URL || "";

    for (const [key, value] of Object.entries(OAUTH_CORS_HEADERS)) {
      event.node.res.setHeader(key, value);
    }
    event.node.res.setHeader("Content-Type", "application/json");

    return {
      resource: straitApiUrl,
      authorization_servers: oidcIssuer ? [oidcIssuer] : [],
      scopes_supported: [...ALL_OAUTH_SCOPES],
    };
  } catch (err) {
    console.error("Failed to serve OAuth protected resource metadata:", err);
    event.node.res.statusCode = 500;
    return { error: "internal_error" };
  }
});
