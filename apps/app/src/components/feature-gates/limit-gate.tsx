import { useSuspenseQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { FEATURE_FLAGS } from "@/hooks/posthog/flags";
import {
  useCanAddMoreByPlan,
  useFeatureLimitByPlan,
} from "@/hooks/posthog/use-feature-flag";
import { useOrgUsage } from "@/hooks/billing/use-org-usage";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";

type AddMoreGateProps = {
  feature: "stores" | "teamMembersPerStore" | "products";
  currentCount: number;
  additionalCount?: number;
  children: ReactNode;
  fallback?: ReactNode;
  upgradePrompt?: (info: {
    limit: number | "unlimited";
    currentCount: number;
    trying: number;
    nextPlan?: { plan: string; name: string };
  }) => React.ReactNode;
};

const FEATURE_TO_FLAG_MAP = {
  stores: FEATURE_FLAGS.LIMIT_STORES,
  teamMembersPerStore: FEATURE_FLAGS.LIMIT_TEAM_MEMBERS_PER_STORE,
  products: FEATURE_FLAGS.LIMIT_PRODUCTS,
} as const;

// Maps feature-gate features to backend usage dimensions for fallback.
const FEATURE_TO_USAGE_MAP = {
  stores: "projects",
  teamMembersPerStore: "members",
  products: "projects",
} as const;

/**
 * Component that checks if user can add more items without exceeding their limit.
 * Uses PostHog feature flags as primary source, with backend usage data as fallback.
 */
export const AddMoreGate = ({
  feature,
  currentCount,
  additionalCount = 1,
  children,
  fallback,
  upgradePrompt,
}: AddMoreGateProps) => {
  const { data } = useSuspenseQuery(subscriptionStateQueryOptions());
  const { plan, isTrialing, nextPlan } = data;
  const { data: orgUsage } = useOrgUsage();

  const flagKey = FEATURE_TO_FLAG_MAP[feature];

  const limit = useFeatureLimitByPlan(flagKey, plan, isTrialing);
  const canAdd = useCanAddMoreByPlan(
    flagKey,
    currentCount,
    plan,
    isTrialing,
    additionalCount
  );

  // Backend fallback: if PostHog returns no limit (-1 or undefined), check
  // the backend usage data directly.
  let effectiveCanAdd = canAdd;
  let effectiveLimit: number | "unlimited" = limit === -1 ? "unlimited" : limit;

  if (limit === -1 && orgUsage) {
    const usageKey = FEATURE_TO_USAGE_MAP[feature];
    const dimension = orgUsage.usage[usageKey];
    if (dimension && dimension.limit > 0) {
      effectiveLimit = dimension.limit;
      effectiveCanAdd = currentCount + additionalCount <= dimension.limit;
    }
  }

  if (effectiveCanAdd) {
    return <>{children}</>;
  }

  if (upgradePrompt) {
    return (
      <>
        {upgradePrompt({
          limit: effectiveLimit,
          currentCount,
          trying: additionalCount,
          nextPlan: nextPlan || undefined,
        })}
      </>
    );
  }

  return fallback ? fallback : null;
};
