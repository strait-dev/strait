/**
 * Global PostHog analytics singleton for non-React contexts.
 *
 * PostHog is initialized lazily in {@link PostHogProvider} (components/providers/posthog-provider.tsx)
 * which wraps the app in router.tsx. On init, it calls {@link setPostHog} to store the
 * instance here so it can be accessed outside React component trees.
 *
 * **Two access patterns:**
 * - **Inside React components**: use `usePostHog()` from `posthog-provider.tsx` or
 *   the typed `useAnalytics()` hook from `hooks/analytics/use-analytics.ts`.
 * - **Outside React** (mutation callbacks, server-adjacent code, event handlers):
 *   use `getPostHog()` from this module. Returns `null` before initialization or
 *   when `VITE_POSTHOG_KEY` is not set.
 *
 * @see https://posthog.com/docs/libraries/js — PostHog JS SDK docs
 * @see https://posthog.com/docs/product-analytics/capture-events — Event capture
 */

type PostHogInstance = typeof import("posthog-js").default;

let _posthog: PostHogInstance | null = null;

/**
 * Get the PostHog client instance.
 *
 * Returns `null` if PostHog hasn't been initialized yet (before first render)
 * or if `VITE_POSTHOG_KEY` is not configured (local dev without analytics).
 *
 * Safe to call at any time -- callers should use optional chaining:
 * ```ts
 * getPostHog()?.capture("event_name", { key: "value" });
 * ```
 */
export const getPostHog = (): PostHogInstance | null => _posthog;

/**
 * Store the PostHog instance after initialization.
 *
 * Called once by {@link PostHogProvider} after `posthog.init()` completes.
 * Should not be called from application code.
 *
 * @internal
 */
export const setPostHog = (instance: PostHogInstance): void => {
  _posthog = instance;
};
