import { defineEventHandler } from "vinxi/http";

export default defineEventHandler(async (event) => {
  try {
    const { auth } = await import("../../../src/lib/auth.server");
    const data = await auth.api.getOpenIdConfig();

    event.node.res.setHeader("Content-Type", "application/json");
    event.node.res.setHeader("Access-Control-Allow-Origin", "*");
    event.node.res.setHeader("Access-Control-Allow-Methods", "GET, OPTIONS");
    event.node.res.setHeader("Cache-Control", "public, max-age=3600");

    return data;
  } catch (err) {
    console.error("Failed to serve OpenID configuration:", err);
    event.node.res.statusCode = 500;
    return { error: "internal_error" };
  }
});
