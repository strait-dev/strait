import type { Plugin } from "vite";
import { defineConfig } from "vitest/config";

/**
 * Virtual-module shim for the `cloudflare:workers` import scheme.
 *
 * `auth.server.ts` imports `env` from `cloudflare:workers` to access the
 * Hyperdrive binding in production. In the Vitest runner the
 * `@cloudflare/vite-plugin` is not active, so Vite's import-analysis
 * would fail to resolve the specifier. This plugin returns an empty-env
 * virtual module, letting tests exercise the fallback path and override
 * it per-case via `vi.doMock("cloudflare:workers", ...)`.
 *
 * Mirrors the production-build shim in `vite.config.ts` (Node target)
 * and the runtime loader shim in `scripts/cloudflare-shim.mjs` (migrate
 * script).
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

export default defineConfig({
  plugins: [shimCloudflareWorkers()],
  resolve: {
    tsconfigPaths: true,
  },
  test: {
    globals: true,
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
    exclude: [
      "**/node_modules/**",
      "**/dist/**",
      "**/cypress/**",
      "**/.{idea,git,cache,output,temp}/**",
      "**/{karma,rollup,webpack,vite,vitest,jest,ava,babel,nyc,cypress,tsup,build}.config.*",
      "**/e2e/**",
      "**/playwright/**",
      "**/*.spec.ts",
      "**/*.spec.tsx",
    ],
  },
});
