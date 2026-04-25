/**
 * Bun preload that shims the `cloudflare:workers` import scheme.
 *
 * Used by the self-host migration runner so `bun scripts/migrate.ts`
 * can load `auth.server.ts` (which imports from `cloudflare:workers`
 * to access the Hyperdrive binding in production) without crashing.
 * In Node, the same job is done by `scripts/cloudflare-shim.mjs` via
 * `node --loader`; Bun has no loader flag, so this uses the plugin API.
 *
 * Mirrors the Vite plugin in `vite.config.ts` and `vitest.config.ts`.
 */
import { plugin } from "bun";

plugin({
  name: "shim-cloudflare-workers",
  setup(build) {
    build.module("cloudflare:workers", () => ({
      loader: "js",
      contents: "export const env = {};\nexport default {};\n",
    }));
  },
});
