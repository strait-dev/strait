import { useSuspenseQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { FEATURE_FLAGS } from "@/hooks/posthog/flags";
import {
  useCanAddMoreByPlan,
  useFeatureLimitByPlan,
} from "@/hooks/posthog/use-feature-flag";
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

/**
 * Component that checks if user can add more items without exceeding their limit
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

  const flagKey = FEATURE_TO_FLAG_MAP[feature];

  const limit = useFeatureLimitByPlan(flagKey, plan, isTrialing);
  const canAdd = useCanAddMoreByPlan(
    flagKey,
    currentCount,
    plan,
    isTrialing,
    additionalCount
  );

  const displayLimit = limit === -1 ? "unlimited" : limit;

  if (canAdd) {
    return <>{children}</>;
  }

  if (upgradePrompt) {
    return (
      <>
        {upgradePrompt({
          limit: displayLimit,
          currentCount,
          trying: additionalCount,
          nextPlan: nextPlan || undefined,
        })}
      </>
    );
  }

  return fallback ? fallback : null;
};
