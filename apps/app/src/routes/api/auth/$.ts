import { createFileRoute } from "@tanstack/react-router";
import { handler } from "@/lib/auth-server";

/**
 * Better Auth route handler for authentication endpoints.
 * Proxies all /api/auth/* routes to the Better Auth handler.
 */
export const Route = createFileRoute("/api/auth/$")({
  server: {
    handlers: {
      GET: ({ request }) => handler(request),
      POST: ({ request }) => handler(request),
    },
  },
});
