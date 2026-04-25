/**
 * Hook for accessing the current organization's plan tier.
 *
 * Derives the plan slug from the org usage data, which is fetched
 * from the Go backend via `/v1/usage/current`.
 *
 * In the community edition, this always returns `"enterprise"` so
 * every feature gate resolves to unlocked. Self-hosters own their
 * infrastructure and there is no billing backing store — locking
 * features in self-host would be user-hostile noise. This keeps
 * `FeatureLock`, `FeatureBadge`, and `canUseFeature` working
 * unchanged without per-site edits.
 */

import { useQuery } from "@tanstack/react-query";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import { isCommunityEdition } from "@/lib/edition";
import type { PlanTierSlug } from "./types";

/**
 * Returns the current organization's plan tier slug.
 *
 * Defaults to `"free"` in cloud when no usage data is available
 * (e.g. unauthenticated or no active organization). Used by feature
 * gates and upgrade prompts.
 *
 * In community edition, always returns `"enterprise"` to unlock
 * every feature gate at the source rather than editing each call
 * site.
 *
 * @returns The current plan tier slug.
 */
export const useCurrentPlan = (): PlanTierSlug => {
  const { data } = useQuery(orgUsageQueryOptions());
  if (isCommunityEdition) {
    return "enterprise";
  }
  return (data?.plan as PlanTierSlug) ?? "free";
};
