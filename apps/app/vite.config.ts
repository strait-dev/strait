import { cloudflare } from "@cloudflare/vite-plugin";
import { sentryTanstackStart } from "@sentry/tanstackstart-react/vite";
import tailwindcss from "@tailwindcss/vite";
import { devtools } from "@tanstack/devtools-vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import viteReact from "@vitejs/plugin-react";
import type { Plugin } from "vite";
import { defineConfig } from "vite";
import { ngrok } from "vite-plugin-ngrok";

const enableNgrok = !!process.env.NGROK_AUTHTOKEN && !process.env.DISABLE_NGROK;

/**
 * Vite plugin that serves /.well-known/oauth-authorization-server and
 * /.well-known/openid-configuration by calling the Better Auth API
 * programmatically. TanStack Start's file router ignores dot-prefixed
 * directories, so these must be handled as server middleware.
 */
function wellKnownOAuthPlugin(): Plugin {
  return {
    name: "well-known-oauth",
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        if (
          req.url !== "/.well-known/oauth-authorization-server" &&
          req.url !== "/.well-known/openid-configuration"
        ) {
          return next();
        }

        try {
          // Dynamic import to avoid loading auth.server.ts at Vite config time
          const { auth } = await server.ssrLoadModule(
            "/src/lib/auth.server.ts"
          );

          let data;
          if (req.url === "/.well-known/oauth-authorization-server") {
            data = await auth.api.getOAuthServerConfig();
          } else {
            data = await auth.api.getOpenIdConfig();
          }

          res.writeHead(200, {
            "Content-Type": "application/json",
            "Access-Control-Allow-Origin": "*",
            "Access-Control-Allow-Methods": "GET, OPTIONS",
            "Cache-Control": "public, max-age=3600",
          });
          res.end(JSON.stringify(data));
        } catch (err) {
          console.error("Failed to serve well-known metadata:", err);
          res.writeHead(500, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ error: "internal_error" }));
        }
      });
    },
  };
}

export default defineConfig({
  resolve: {
    tsconfigPaths: true,
  },
  plugins: [
    cloudflare({
      configPath: "../../wrangler.jsonc",
      viteEnvironment: { name: "ssr" },
    }),
    wellKnownOAuthPlugin(),
    devtools(),
    tailwindcss(),
    tanstackStart({
      router: {
        routeToken: "layout",
      },
      srcDirectory: "src",
    }),
    viteReact(),
    sentryTanstackStart({
      org: process.env.SENTRY_ORG,
      project: process.env.SENTRY_PROJECT,
      authToken: process.env.SENTRY_AUTH_TOKEN,
    }),
    ...(enableNgrok ? [ngrok()] : []),
  ],
  optimizeDeps: {
    include: ["@hugeicons/react"],
    // Exclude PGlite from optimization - it's a test-only dependency
    exclude: ["@electric-sql/pglite", "drizzle-orm/pglite"],
  },
  build: {
    rollupOptions: {
      // Externalize PGlite and test-only dependencies to prevent bundling
      // PGlite is a 20MB in-memory PostgreSQL used only for API tests
      external: [
        "@electric-sql/pglite",
        "drizzle-orm/pglite",
        "drizzle-orm/pglite/migrator",
      ],
    },
  },
  server: {
    port: 5173,
    host: true,
  },
});
