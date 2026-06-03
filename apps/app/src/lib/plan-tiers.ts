const TIER_RANK: Record<string, number> = {
  free: 0,
  starter: 1,
  pro: 2,
  scale: 3,
  business: 4,
  enterprise: 5,
};

const TIER_LABEL: Record<string, string> = {
  free: "Free",
  starter: "Starter",
  pro: "Pro",
  scale: "Scale",
  business: "Business",
  enterprise: "Enterprise",
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
  | "sla"
  | "dedicated_compute"
  | "static_ips"
  | "vpc_peering"
  | "scim"
  | "data_residency"
  | "custom_rbac"
  | "ip_allowlisting"
  | "session_management"
  | "secret_rotation"
  | "siem_export";

/** Minimum tier required for launch-active features. */
const FEATURE_MIN_TIER: Partial<Record<PlanFeature, string>> = {
  http_mode: "pro",
  approval_gates: "pro",
  sub_workflows: "pro",
  job_chaining: "pro",
  compensating_txns: "pro",
  canary_deployments: "scale",
  audit_logs: "scale",
  sla: "business",
};

const ROADMAP_FEATURES = new Set<PlanFeature>([
  "sso",
  "dedicated_compute",
  "static_ips",
  "vpc_peering",
  "scim",
  "data_residency",
  "custom_rbac",
  "ip_allowlisting",
  "session_management",
  "secret_rotation",
  "siem_export",
]);

/** Returns true when a feature is shown for roadmap/contact-sales only. */
export function isRoadmapFeature(feature: PlanFeature): boolean {
  return ROADMAP_FEATURES.has(feature);
}

/** Returns true if `tier` has access to the given feature. */
export function canUseFeature(
  tier: string | undefined,
  feature: PlanFeature
): boolean {
  const minTier = FEATURE_MIN_TIER[feature];
  if (isRoadmapFeature(feature)) {
    return false;
  }
  if (!minTier) {
    return true;
  }
  return tierAtLeast(tier, minTier);
}

/** Returns the human-readable minimum tier label for a gated feature. */
export function getFeatureMinimumPlanLabel(feature: PlanFeature): string {
  if (isRoadmapFeature(feature)) {
    return "Roadmap";
  }
  const minTier = FEATURE_MIN_TIER[feature];
  if (!minTier) {
    return "Free";
  }
  return TIER_LABEL[minTier] ?? minTier;
}
