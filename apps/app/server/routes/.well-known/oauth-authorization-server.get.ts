import { defineEventHandler } from "vinxi/http";

export default defineEventHandler(async (event) => {
  try {
    const [{ auth }, { OAUTH_CORS_HEADERS }] = await Promise.all([
      import("../../../src/lib/auth.server"),
      import("../../../src/lib/oauth-scopes"),
    ]);
    const data = await auth.api.getOAuthServerConfig();

    for (const [key, value] of Object.entries(OAUTH_CORS_HEADERS)) {
      event.node.res.setHeader(key, value);
    }
    event.node.res.setHeader("Content-Type", "application/json");

    return data;
  } catch (err) {
    console.error("Failed to serve OAuth authorization server metadata:", err);
    event.node.res.statusCode = 500;
    return { error: "internal_error" };
  }
});
