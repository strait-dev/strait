import { usePostHog } from "@/components/providers/posthog-provider";
import type { FeatureFlagKey } from "./flags";

type PlanKey = "free" | "starter" | "pro" | "enterprise" | "trial";
type LimitPayload = Record<PlanKey, number>;

/**
 * Check if a boolean feature flag is enabled.
 * @param flagKey - Type-safe feature flag key from FEATURE_FLAGS
 * @returns true if enabled, false otherwise (defaults to true if PostHog unavailable)
 */
export const useFeatureFlag = (flagKey: FeatureFlagKey): boolean => {
  const posthog = usePostHog();

  // During SSR, PostHog methods don't exist - return permissive default
  if (!posthog || typeof posthog.isFeatureEnabled !== "function") {
    return true; // Default permissive if PostHog unavailable
  }

  return posthog.isFeatureEnabled(flagKey) ?? true;
};

/**
 * Get numeric limit from a feature flag payload.
 * Payload should be a JSON object mapping plan names to limits.
 * Example: { "free": 1, "starter": 2, "pro": 3, "enterprise": 5, "trial": 5 }
 *
 * @param flagKey - Type-safe feature flag key from FEATURE_FLAGS
 * @param plan - The user's current plan
 * @param isTrialing - Whether the user is currently in trial
 * @returns The numeric limit, or -1 for unlimited
 */
export const useFeatureLimitByPlan = (
  flagKey: FeatureFlagKey,
  plan: string,
  isTrialing = false
): number => {
  const posthog = usePostHog();

  // During SSR, PostHog methods don't exist - return permissive default
  if (!posthog || typeof posthog.getFeatureFlagPayload !== "function") {
    return -1; // Default unlimited if PostHog unavailable
  }

  const payload = posthog.getFeatureFlagPayload(flagKey) as LimitPayload | null;

  if (!payload || typeof payload !== "object") {
    return -1; // Default unlimited if no payload
  }

  // Trial users get trial limit (or enterprise if no trial key)
  if (isTrialing) {
    return payload.trial ?? payload.enterprise ?? -1;
  }

  // Look up by plan
  const planKey = plan.toLowerCase() as PlanKey;
  const limit = payload[planKey];

  if (typeof limit === "number") {
    return limit;
  }

  return -1; // Default unlimited
};

/**
 * Get numeric limit from a feature flag payload (legacy, direct value).
 * Use useFeatureLimitByPlan for JSON payload lookups.
 * @param flagKey - Type-safe feature flag key from FEATURE_FLAGS
 * @returns The numeric limit, or -1 for unlimited
 */
const useFeatureLimit = (flagKey: FeatureFlagKey): number => {
  const posthog = usePostHog();

  // During SSR, PostHog methods don't exist - return permissive default
  if (!posthog || typeof posthog.getFeatureFlagPayload !== "function") {
    return -1; // Default unlimited if PostHog unavailable
  }

  const payload = posthog.getFeatureFlagPayload(flagKey);

  if (typeof payload === "number") {
    return payload;
  }

  if (typeof payload === "string") {
    const parsed = Number.parseInt(payload, 10);
    return Number.isNaN(parsed) ? -1 : parsed;
  }

  return -1; // Default unlimited
};

/**
 * Get tiered feature value from a feature flag payload.
 * @param flagKey - Type-safe feature flag key from FEATURE_FLAGS
 * @returns The tier string value, or "none" if unavailable
 */
// biome-ignore lint/correctness/noUnusedVariables: Kept for future PostHog feature flag usage
const useFeatureTier = (flagKey: FeatureFlagKey): string => {
  const posthog = usePostHog();

  // During SSR, PostHog methods don't exist - return default
  if (!posthog || typeof posthog.getFeatureFlagPayload !== "function") {
    return "none";
  }

  const payload = posthog.getFeatureFlagPayload(flagKey);
  return typeof payload === "string" ? payload : "none";
};

/**
 * Check if a limit has been reached (plan-based).
 * @param flagKey - Type-safe feature flag key from FEATURE_FLAGS
 * @param currentCount - Current count of items
 * @param plan - The user's current plan
 * @param isTrialing - Whether the user is currently in trial
 * @returns true if limit reached, false if can add more
 */
// biome-ignore lint/correctness/noUnusedVariables: Kept for future PostHog feature flag usage
const useHasReachedLimitByPlan = (
  flagKey: FeatureFlagKey,
  currentCount: number,
  plan: string,
  isTrialing = false
): boolean => {
  const limit = useFeatureLimitByPlan(flagKey, plan, isTrialing);
  if (limit === -1) {
    return false; // Unlimited
  }
  return currentCount >= limit;
};

/**
 * Check if N more items can be added within the limit (plan-based).
 * @param flagKey - Type-safe feature flag key from FEATURE_FLAGS
 * @param currentCount - Current count of items
 * @param plan - The user's current plan
 * @param isTrialing - Whether the user is currently in trial
 * @param toAdd - Number of items to add (default 1)
 * @returns true if can add, false if would exceed limit
 */
export const useCanAddMoreByPlan = (
  flagKey: FeatureFlagKey,
  currentCount: number,
  plan: string,
  isTrialing = false,
  toAdd = 1
): boolean => {
  const limit = useFeatureLimitByPlan(flagKey, plan, isTrialing);
  if (limit === -1) {
    return true; // Unlimited
  }
  return currentCount + toAdd <= limit;
};

/**
 * Check if a limit has been reached (legacy, direct value).
 * @param flagKey - Type-safe feature flag key from FEATURE_FLAGS
 * @param currentCount - Current count of items
 * @returns true if limit reached, false if can add more
 */
// biome-ignore lint/correctness/noUnusedVariables: Kept for future PostHog feature flag usage
const useHasReachedLimit = (
  flagKey: FeatureFlagKey,
  currentCount: number
): boolean => {
  const limit = useFeatureLimit(flagKey);
  if (limit === -1) {
    return false; // Unlimited
  }
  return currentCount >= limit;
};

/**
 * Check if N more items can be added within the limit (legacy, direct value).
 * @param flagKey - Type-safe feature flag key from FEATURE_FLAGS
 * @param currentCount - Current count of items
 * @param toAdd - Number of items to add (default 1)
 * @returns true if can add, false if would exceed limit
 */
// biome-ignore lint/correctness/noUnusedVariables: Kept for future PostHog feature flag usage
const useCanAddMore = (
  flagKey: FeatureFlagKey,
  currentCount: number,
  toAdd = 1
): boolean => {
  const limit = useFeatureLimit(flagKey);
  if (limit === -1) {
    return true; // Unlimited
  }
  return currentCount + toAdd <= limit;
};
