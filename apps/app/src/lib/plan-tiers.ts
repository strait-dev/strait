const TIER_RANK: Record<string, number> = {
  free: 0,
  starter: 1,
  pro: 2,
  scale: 3,
  enterprise: 4,
};

/** Returns true if switching from `currentTier` to `targetTier` is a downgrade. */
export function isDowngrade(
  currentTier: string | undefined,
  targetTier: string | undefined
): boolean {
  if (!(currentTier && targetTier)) {
    return false;
  }
  return (TIER_RANK[targetTier] ?? 0) < (TIER_RANK[currentTier] ?? 0);
}

/** Returns true if `tier` is at or above `minimumTier` in the plan hierarchy. */
export function tierAtLeast(
  tier: string | undefined,
  minimumTier: string
): boolean {
  if (!tier) {
    return false;
  }
  return (TIER_RANK[tier] ?? 0) >= (TIER_RANK[minimumTier] ?? 0);
}

/** Plan-gated feature identifiers (mirrors Go billing.Feature). */
export type PlanFeature =
  | "http_mode"
  | "approval_gates"
  | "sub_workflows"
  | "job_chaining"
  | "compensating_txns"
  | "canary_deployments"
  | "audit_logs"
  | "sso"
  | "sla";

/** Minimum tier required for each feature. */
const FEATURE_MIN_TIER: Record<PlanFeature, string> = {
  http_mode: "pro",
  approval_gates: "pro",
  sub_workflows: "pro",
  job_chaining: "pro",
  compensating_txns: "pro",
  canary_deployments: "scale",
  audit_logs: "scale",
  sso: "enterprise",
  sla: "enterprise",
};

/** Returns true if `tier` has access to the given feature. */
export function canUseFeature(
  tier: string | undefined,
  feature: PlanFeature
): boolean {
  const minTier = FEATURE_MIN_TIER[feature];
  if (!minTier) {
    return true;
  }
  return tierAtLeast(tier, minTier);
}
