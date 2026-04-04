/**
 * Hook for accessing the current organization's plan tier.
 *
 * Derives the plan slug from the org usage data, which is fetched
 * from the Go backend via `/v1/usage/current`.
 */

import { useQuery } from "@tanstack/react-query";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import type { PlanTierSlug } from "./types";

/**
 * Returns the current organization's plan tier slug.
 *
 * Defaults to `"free"` when no usage data is available (e.g. unauthenticated
 * or no active organization). Used by feature gates and upgrade prompts.
 *
 * @returns The current plan tier slug.
 */
export const useCurrentPlan = (): PlanTierSlug => {
  const { data } = useQuery(orgUsageQueryOptions());
  return (data?.plan as PlanTierSlug) ?? "free";
};
