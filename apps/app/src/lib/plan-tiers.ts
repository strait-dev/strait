import {
  PLAN_API_RESPONSE,
  PLAN_KEYS,
  PLANS,
  type PlanApiResponse,
  type PlanKey,
  ROADMAP_FEATURES,
} from "@strait/billing";

const TIER_RANK = Object.fromEntries(
  PLAN_KEYS.map((tier, index) => [tier, index])
) as Record<PlanKey, number>;

const PLAN_BY_TIER = Object.fromEntries(
  PLAN_API_RESPONSE.map((plan) => [plan.tier, plan])
) as Record<PlanKey, PlanApiResponse>;

const isPlanKey = (tier: string | undefined): tier is PlanKey =>
  !!tier && PLAN_KEYS.includes(tier as PlanKey);

const rankForTier = (tier: string | undefined): number | undefined =>
  isPlanKey(tier) ? TIER_RANK[tier] : undefined;

/** Returns true if switching from `currentTier` to `targetTier` is a downgrade. */
export function isDowngrade(
  currentTier: string | undefined,
  targetTier: string | undefined
): boolean {
  if (!(currentTier && targetTier)) {
    return false;
  }
  return (rankForTier(targetTier) ?? 0) < (rankForTier(currentTier) ?? 0);
}

/** Returns true if `tier` is at or above `minimumTier` in the plan hierarchy. */
export function tierAtLeast(
  tier: string | undefined,
  minimumTier: string
): boolean {
  if (!tier) {
    return false;
  }
  return (rankForTier(tier) ?? 0) >= (rankForTier(minimumTier) ?? 0);
}

/** Plan-gated feature identifiers shown by the app. */
export type PlanFeature =
  | "http_mode"
  | "log_streaming"
  | "approval_gates"
  | "sub_workflows"
  | "job_chaining"
  | "compensating_txns"
  | "canary_deployments"
  | "audit_logs"
  | "sso"
  | "sla"
  | "dedicated_worker_pool"
  | "static_ips"
  | "vpc_peering"
  | "scim"
  | "data_residency"
  | "ip_allowlisting"
  | "single_tenant"
  | "byo_cloud"
  | "compliance_archive";

const FEATURE_ACCESSORS: Partial<
  Record<PlanFeature, (plan: PlanApiResponse) => boolean>
> = {
  http_mode: (plan) =>
    PLANS[plan.tier].limits.executionModes.toLowerCase().includes("http"),
  log_streaming: (plan) => plan.has_log_streaming,
  approval_gates: (plan) => plan.has_approval_gates,
  sub_workflows: (plan) => plan.has_sub_workflows,
  job_chaining: (plan) => plan.has_job_chaining,
  compensating_txns: (plan) => plan.has_compensating_txns,
  canary_deployments: (plan) => plan.has_canary_deployments,
  audit_logs: (plan) => plan.has_audit_logs,
  sla: (plan) => plan.has_sla,
};

export const ROADMAP_FEATURE_LABELS: Partial<Record<PlanFeature, string>> = {
  sso: "SSO/SAML",
  dedicated_worker_pool: "dedicated worker pool",
  static_ips: "static IPs",
  vpc_peering: "VPC peering",
  scim: "SCIM",
  data_residency: "data residency",
  ip_allowlisting: "IP allowlisting",
  single_tenant: "single-tenant orchestration",
  byo_cloud: "BYO-cloud",
  compliance_archive: "compliance archive",
};

/** Returns true when a feature is shown for roadmap/contact-sales only. */
export function isRoadmapFeature(feature: PlanFeature): boolean {
  const label = ROADMAP_FEATURE_LABELS[feature];
  return !!label && ROADMAP_FEATURES.includes(label);
}

/** Returns true if `tier` has access to the given feature. */
export function canUseFeature(
  tier: string | undefined,
  feature: PlanFeature
): boolean {
  if (isRoadmapFeature(feature)) {
    return false;
  }
  if (!isPlanKey(tier)) {
    return false;
  }
  const accessor = FEATURE_ACCESSORS[feature];
  return accessor ? accessor(PLAN_BY_TIER[tier]) : false;
}

/** Returns the human-readable minimum tier label for a gated feature. */
export function getFeatureMinimumPlanLabel(feature: PlanFeature): string {
  if (isRoadmapFeature(feature)) {
    return "Roadmap";
  }
  const accessor = FEATURE_ACCESSORS[feature];
  if (!accessor) {
    return "Unavailable";
  }
  const minimumTier = PLAN_KEYS.find((tier) => accessor(PLAN_BY_TIER[tier]));
  return minimumTier ? PLANS[minimumTier].name : "Unavailable";
}
