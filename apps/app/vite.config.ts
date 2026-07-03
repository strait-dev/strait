import { fileURLToPath } from "node:url";
import { cloudflare } from "@cloudflare/vite-plugin";
import { sentryTanstackStart } from "@sentry/tanstackstart-react/vite";
import tailwindcss from "@tailwindcss/vite";
import { devtools } from "@tanstack/devtools-vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import viteReact from "@vitejs/plugin-react";
import { nitro } from "nitro/vite";
import type { Plugin } from "vite";
import { defineConfig } from "vite";
import { ngrok } from "vite-plugin-ngrok";
import { resolveSentryRelease } from "./scripts/sentry-release";

const enableNgrok =
  process.env.ENABLE_NGROK === "1" && !process.env.DISABLE_NGROK;

/**
 * Build target selector. Defaults to portable Node output. Cloudflare and
 * Vercel are explicit secondary targets for hosted deployments.
 */
type DeployTarget = "cloudflare" | "node" | "vercel";

function resolveDeployTarget(env: NodeJS.ProcessEnv): DeployTarget {
  if (
    env.STRAIT_APP_TARGET === "cloudflare" ||
    env.BUILD_TARGET === "cloudflare"
  ) {
    return "cloudflare";
  }
  if (env.STRAIT_APP_TARGET === "vercel" || env.BUILD_TARGET === "vercel") {
    return "vercel";
  }
  return "node";
}

const deployTarget = resolveDeployTarget(process.env);
const isCloudflareBuild = deployTarget === "cloudflare";
const nitroPreset = deployTarget === "vercel" ? "vercel" : "node-server";

const emitSourcemapsForSentry = process.env.SENTRY_UPLOAD_SOURCEMAPS === "true";

const sentryRelease = maybeResolveSentryRelease(process.env);

function maybeResolveSentryRelease(env: NodeJS.ProcessEnv): string | undefined {
  try {
    return resolveSentryRelease(env);
  } catch {
    return;
  }
}

/**
 * Vite plugin that serves OAuth well-known metadata by calling the Better Auth API
 * programmatically. TanStack Start's file router ignores dot-prefixed
 * directories, so these must be handled as server middleware.
 */
function wellKnownOAuthPlugin(): Plugin {
  const oauthServerConfigPaths = new Set([
    "/.well-known/oauth-authorization-server",
    "/.well-known/oauth-authorization-server/api/auth",
  ]);
  const openIdConfigPaths = new Set([
    "/.well-known/openid-configuration",
    "/api/auth/.well-known/openid-configuration",
  ]);

  return {
    name: "well-known-oauth",
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        const path = req.url?.split("?")[0] ?? "";
        if (
          !(oauthServerConfigPaths.has(path) || openIdConfigPaths.has(path))
        ) {
          return next();
        }

        try {
          // Dynamic import to avoid loading auth.server.ts at Vite config time
          const { getAuth } = await server.ssrLoadModule(
            "/src/lib/auth.server.ts"
          );
          const auth = await getAuth();

          const data = oauthServerConfigPaths.has(path)
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

export default defineConfig(({ command }) => ({
  resolve: {
    alias: {
      "decimal.js-light": fileURLToPath(
        new URL(
          "../../node_modules/.bun/decimal.js-light@2.5.1/node_modules/decimal.js-light/decimal.mjs",
          import.meta.url
        )
      ),
      eventemitter3: fileURLToPath(
        new URL(
          "../../node_modules/.bun/eventemitter3@5.0.4/node_modules/eventemitter3/index.mjs",
          import.meta.url
        )
      ),
      "react-is": fileURLToPath(
        new URL("./src/lib/react-is.ts", import.meta.url)
      ),
      "es-toolkit/compat/get": fileURLToPath(
        new URL("./src/lib/es-toolkit-compat-get.ts", import.meta.url)
      ),
      "es-toolkit/compat/isPlainObject": fileURLToPath(
        new URL(
          "./src/lib/es-toolkit-compat-is-plain-object.ts",
          import.meta.url
        )
      ),
      "es-toolkit/compat/last": fileURLToPath(
        new URL("./src/lib/es-toolkit-compat-last.ts", import.meta.url)
      ),
      "es-toolkit/compat/maxBy": fileURLToPath(
        new URL("./src/lib/es-toolkit-compat-max-by.ts", import.meta.url)
      ),
      "es-toolkit/compat/minBy": fileURLToPath(
        new URL("./src/lib/es-toolkit-compat-min-by.ts", import.meta.url)
      ),
      "es-toolkit/compat/omit": fileURLToPath(
        new URL("./src/lib/es-toolkit-compat-omit.ts", import.meta.url)
      ),
      "es-toolkit/compat/range": fileURLToPath(
        new URL("./src/lib/es-toolkit-compat-range.ts", import.meta.url)
      ),
      "es-toolkit/compat/sortBy": fileURLToPath(
        new URL("./src/lib/es-toolkit-compat-sort-by.ts", import.meta.url)
      ),
      "es-toolkit/compat/sumBy": fileURLToPath(
        new URL("./src/lib/es-toolkit-compat-sum-by.ts", import.meta.url)
      ),
      "es-toolkit/compat/throttle": fileURLToPath(
        new URL("./src/lib/es-toolkit-compat-throttle.ts", import.meta.url)
      ),
      "es-toolkit/compat/uniqBy": fileURLToPath(
        new URL("./src/lib/es-toolkit-compat-uniq-by.ts", import.meta.url)
      ),
      "use-sync-external-store/with-selector": fileURLToPath(
        new URL(
          "./src/lib/use-sync-external-store-with-selector.ts",
          import.meta.url
        )
      ),
      "use-sync-external-store/with-selector.js": fileURLToPath(
        new URL(
          "./src/lib/use-sync-external-store-with-selector.ts",
          import.meta.url
        )
      ),
    },
    tsconfigPaths: true,
  },
  plugins: [
    ...(isCloudflareBuild
      ? [cloudflare({ viteEnvironment: { name: "ssr" } })]
      : []),
    wellKnownOAuthPlugin(),
    ...(command === "serve" ? [devtools()] : []),
    tailwindcss(),
    tanstackStart({
      router: {
        routeToken: "layout",
      },
      srcDirectory: "src",
    }),
    ...(isCloudflareBuild ? [] : [nitro({ preset: nitroPreset })]),
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
    entries: ["src/**/*.{ts,tsx}", "!src/**/*.test.{ts,tsx}"],
    include: [
      "@hugeicons/react",
      "defu",
      "jose/errors",
      "use-sync-external-store/shim",
      "use-sync-external-store/shim/with-selector",
    ],
    exclude: [
      // Keep the design-system package out of Vite's prebundle cache. Its
      // many subpath exports can otherwise be invalidated mid-reload in dev.
      "@strait/ui",
      "@electric-sql/pglite",
      "drizzle-orm/pglite",
    ],
  },
  build: {
    sourcemap: emitSourcemapsForSentry,
    rollupOptions: {
      output: {
        assetFileNames(assetInfo) {
          if (assetInfo.names.some((name) => name.endsWith(".css"))) {
            return "assets/[name][extname]";
          }

          return "assets/[name]-[hash][extname]";
        },
      },
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
    host: "localhost",
    strictPort: true,
  },
}));
