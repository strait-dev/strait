import { defineEventHandler } from "vinxi/http";

export default defineEventHandler(async (event) => {
  try {
    const { auth } = await import("../../../src/lib/auth.server");
    const { OAUTH_CORS_HEADERS } = await import(
      "../../../src/lib/oauth-scopes"
    );
    const data = await auth.api.getOpenIdConfig();

    for (const [key, value] of Object.entries(OAUTH_CORS_HEADERS)) {
      event.node.res.setHeader(key, value);
    }
    event.node.res.setHeader("Content-Type", "application/json");

    return data;
  } catch (err) {
    console.error("Failed to serve OpenID configuration:", err);
    event.node.res.statusCode = 500;
    return { error: "internal_error" };
  }
});
