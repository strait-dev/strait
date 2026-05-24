import { cloudflare } from "@cloudflare/vite-plugin";
import { sentryTanstackStart } from "@sentry/tanstackstart-react/vite";
import tailwindcss from "@tailwindcss/vite";
import { devtools } from "@tanstack/devtools-vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import viteReact from "@vitejs/plugin-react";
import type { Plugin } from "vite";
import { defineConfig } from "vite";
import { ngrok } from "vite-plugin-ngrok";
import { resolveSentryRelease } from "./scripts/sentry-release";

const enableNgrok = !!process.env.NGROK_AUTHTOKEN && !process.env.DISABLE_NGROK;

/**
 * Build target selector. Defaults to `cloudflare` so `vite build` and the
 * strait.dev production deploy remain unchanged. `BUILD_TARGET=node` drops
 * the Cloudflare plugin and produces a self-contained SSR bundle at
 * `dist/server/server.js`, wrapped for HTTP by `scripts/node-server.mjs`
 * and used by the self-host Docker image.
 */
const buildTarget: "cloudflare" | "node" =
  process.env.BUILD_TARGET === "node" ? "node" : "cloudflare";

const sentryRelease = maybeResolveSentryRelease(process.env);

function maybeResolveSentryRelease(env: NodeJS.ProcessEnv): string | undefined {
  try {
    return resolveSentryRelease(env);
  } catch {
    return;
  }
}

/**
 * Virtual-module shim for the `cloudflare:workers` import scheme.
 *
 * Used only when `BUILD_TARGET=node`. `auth.server.ts` imports `env` from
 * `cloudflare:workers` to access the Hyperdrive binding; in Node that
 * specifier does not exist and would crash the bundle. The resolver
 * catches the specifier and returns an empty-env virtual module, letting
 * `getAuthConnectionString()` fall through to `process.env.AUTH_DATABASE_URL`.
 *
 * Also needed: the Node SSR build externalizes `node_modules` by default,
 * which breaks named imports from CommonJS packages (e.g. `@opentelemetry/
 * semantic-conventions`). The Node branch sets `ssr.noExternal: true` below
 * to force inline bundling, making the output self-contained.
 *
 * Mirrors the runtime loader shim at `apps/app/scripts/cloudflare-shim.mjs`
 * used by the migrate script.
 */
function shimCloudflareWorkers(): Plugin {
  const virtualId = "\0virtual:cloudflare-workers";
  return {
    name: "shim-cloudflare-workers",
    enforce: "pre",
    resolveId(id) {
      if (id === "cloudflare:workers") {
        return virtualId;
      }
      return null;
    },
    load(id) {
      if (id === virtualId) {
        return "export const env = {};\nexport default {};\n";
      }
      return null;
    },
  };
}

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

          const data =
            req.url === "/.well-known/oauth-authorization-server"
              ? await auth.api.getOAuthServerConfig()
              : await auth.api.getOpenIdConfig();

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
    ...(buildTarget === "cloudflare"
      ? [cloudflare({ viteEnvironment: { name: "ssr" } })]
      : [shimCloudflareWorkers()]),
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
      ...(sentryRelease
        ? {
            release: {
              name: sentryRelease,
              inject: true,
              create: false,
              finalize: false,
            },
          }
        : {}),
    }),
    ...(enableNgrok ? [ngrok()] : []),
  ],
  optimizeDeps: {
    include: ["@hugeicons/react"],
    // Exclude PGlite from optimization - it's a test-only dependency
    exclude: ["@electric-sql/pglite", "drizzle-orm/pglite"],
  },
  build: {
    sourcemap: true,
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
  // Node target: inline-bundle all deps so `node dist/server/server.js`
  // is self-contained. Without this, Vite leaves node_modules as
  // externals and Node's ESM loader fails on CJS named imports.
  ...(buildTarget === "node"
    ? {
        ssr: {
          noExternal: true,
        },
      }
    : {}),
  server: {
    port: 5173,
    host: true,
  },
});
