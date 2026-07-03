/**
 * Strait edition — compile-time flag that gates cloud-only features
 * (billing, Stripe checkout, customer portal, upgrade nudges) out of
 * the self-host Docker image and Vercel deploy flow.
 *
 * The flag is set at build time via the `VITE_STRAIT_EDITION` env var.
 * Vite inlines `import.meta.env.VITE_STRAIT_EDITION` into both the
 * client and the SSR bundle, so every read gets constant-folded down
 * to a literal and the dead branches are tree-shaken out.
 *
 * **Default is `cloud`** so the strait.dev production deploy keeps
 * billing on without needing to set any new env var. The self-host
 * Dockerfile explicitly sets `VITE_STRAIT_EDITION=community` at build time.
 *
 * **Defense in depth:** this module gates UI (sidebar link, route
 * redirects, hidden banners) _and_ server functions (checkout /
 * customer portal / Stripe customer creation throw on community).
 * Even if the sidebar was bypassed, the mutations refuse to run.
 */

type Edition = "cloud" | "community";

const viteEnv = (
  import.meta as ImportMeta & {
    env?: { VITE_STRAIT_EDITION?: string };
  }
).env;

const RAW_EDITION: string | undefined =
  viteEnv?.VITE_STRAIT_EDITION ??
  (typeof process === "undefined"
    ? undefined
    : process.env.VITE_STRAIT_EDITION);

const EDITION: Edition = RAW_EDITION === "community" ? "community" : "cloud";

export const isCommunityEdition = EDITION === "community";

/**
 * Throws with a consistent message when a Stripe-touching server
 * function is invoked in community mode. Used by `startCheckoutServerFn`,
 * `startAddonCheckoutServerFn`, `getCustomerPortalUrlServerFn`, etc.
 */
export const assertCloudEdition = (action: string): void => {
  if (isCommunityEdition) {
    throw new Error(
      `${action} is not available in Strait community edition. ` +
        "Billing and Stripe integration are cloud-only features."
    );
  }
};
