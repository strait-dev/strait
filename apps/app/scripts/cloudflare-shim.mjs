/**
 * Node.js loader hook that shims `cloudflare:*` imports with empty modules.
 *
 * Used by the migrate script which imports auth.server.ts (which imports
 * `cloudflare:workers`). In Node, Hyperdrive is unavailable so the code
 * falls back to AUTH_DATABASE_URL anyway — we just need the import to
 * not crash.
 */
export async function resolve(specifier, context, nextResolve) {
  if (specifier.startsWith("cloudflare:")) {
    return {
      shortCircuit: true,
      url: `data:application/javascript,export default {};export const env = {};`,
    };
  }
  return nextResolve(specifier, context);
}
