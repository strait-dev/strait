/**
 * Plan definitions and formatting utilities.
 *
 * Fetches plan tier data from the Go backend via `GET /v1/plans` and
 * transforms it into UI-friendly structures for the pricing page,
 * plan comparison table, and upgrade flow.
 */

import { createServerFn } from "@tanstack/react-start";
import { apiRequest } from "@/lib/api-client.server";

export {
  formatBoolean,
  formatComputeCredit,
  formatLimit,
  formatRBAC,
  formatRegionCount,
  formatRetention,
  formatSupportLevel,
} from "./plan-formatters";

import {
  formatBoolean,
  formatComputeCredit,
  formatLimit,
  formatRBAC,
  formatRegionCount,
  formatRetention,
  formatSupportLevel,
} from "./plan-formatters";

/** Raw plan data from the `GET /v1/plans` API response. */
export type APIPlan = {
  tier: string;
  display_name: string;
  price_monthly_usd: number;
  price_annual_usd: number;
  max_orgs_per_user: number;
  max_projects_per_org: number;
  max_members_per_org: number;
  max_runs_per_day: number;
  max_concurrent_runs: number;
  compute_credit_microusd: number;
  free_managed_runs_per_month: number;
  free_managed_preset: string;
  free_managed_max_timeout: number;
  retention_days: number;
  allowed_regions: string[];
  max_alert_rules_per_project: number;
  max_webhook_subs_per_project: number;
  max_log_drains_per_org: number;
  max_ai_model_calls_per_day: number;
  ai_assistant_byok: boolean;
  has_rbac: boolean;
  rbac_level: string;
  has_audit_logs: boolean;
  has_sso: boolean;
  has_sla: boolean;
  requires_credit_card: boolean;
  overage_per_k_runs_microusd: number;
  support_level: string;
  has_dedicated_compute: boolean;
  has_static_ips: boolean;
  has_vpc_peering: boolean;
  has_scim: boolean;
  has_data_residency: boolean;
  has_custom_rbac: boolean;
  has_reserved_capacity: boolean;
  has_priority_queue: boolean;
  has_ip_allowlisting: boolean;
  has_session_management: boolean;
  has_secret_rotation: boolean;
  has_siem_export: boolean;
};

/** API response wrapper for the plans endpoint. */
type PlansResponse = {
  plans: APIPlan[];
};

/**
 * Server function to fetch all plan tier definitions from the Go backend.
 *
 * @returns Array of plan definitions ordered by tier rank.
 */
export const getPlansServerFn = createServerFn({ method: "GET" }).handler(
  async () => {
    const data = await apiRequest<PlansResponse>("/v1/plans");
    return data.plans;
  }
);

/** A single feature listed on a pricing card. */
export type PricingFeature = {
  /** Feature name for display. */
  name: string;
  /** Optional longer description. */
  description?: string;
  /** Whether the feature is included in the plan. */
  included: boolean;
};

/** Pricing plan data shaped for the upgrade page plan cards. */
export type PricingPlan = {
  /** Display name (e.g. "Pro", "Scale"). */
  name: string;
  /** Plan tier slug. */
  slug: "free" | "starter" | "pro" | "scale" | "enterprise";
  /** Short description of the plan's target audience. */
  description: string;
  /** Plan prices in cents. */
  prices: {
    monthly: number;
    yearly: number;
    monthlyInYearly?: number;
  };
  /** Features listed on the pricing card. */
  features: PricingFeature[];
  /** Whether this plan card should be visually highlighted. */
  highlight?: boolean;
  /** Optional badge text (e.g. "Most popular"). */
  badge?: string;
  /** Badge visual variant. */
  badgeVariant?: "success-light" | "info-light" | "default";
  /** Whether the plan uses custom pricing (Enterprise). */
  isCustomPricing?: boolean;
};

/** Plan descriptions for the pricing cards. */
const PLAN_DESCRIPTIONS: Record<string, string> = {
  free: "For side projects and evaluation. All features included.",
  starter: "For growing teams with production workloads.",
  pro: "For production workloads at scale.",
  scale: "For high-volume teams that need audit logs and canary deploys.",
  enterprise: "Custom everything for large organizations.",
};

/**
 * Transform API plan data into pricing plan objects for the upgrade page.
 *
 * Enterprise plans receive a special feature list and custom pricing flag.
 * Other plans get their features derived from the API response fields.
 *
 * @param plans - Array of raw API plan data.
 * @returns Array of pricing plans shaped for the plan selection UI.
 */
export const apiPlansToPricingPlans = (plans: APIPlan[]): PricingPlan[] => {
  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: feature mapping requires many branches per plan tier
  return plans.map((p) => {
    const slug = p.tier as PricingPlan["slug"];
    const isEnterprise = slug === "enterprise";
    const isFree = slug === "free";

    const features: PricingFeature[] = [
      { name: "All core features", included: true },
    ];

    if (isEnterprise) {
      features.push(
        { name: "Unlimited everything", included: true },
        { name: "Custom compute credits", included: true },
        { name: "SSO/SAML", included: true },
        { name: "99.9% SLA", included: true },
        {
          name: `${formatRetention(p.retention_days)} retention`,
          included: true,
        },
        { name: formatSupportLevel(p.support_level), included: true },
        { name: "Custom integrations", included: true },
        { name: "Static IPs", included: true }
      );
    } else {
      features.push(
        {
          name: `${formatLimit(p.max_runs_per_day)} runs/day`,
          included: true,
        },
        ...(p.free_managed_runs_per_month > 0
          ? [
              {
                name: `${p.free_managed_runs_per_month} managed runs/mo (${p.free_managed_preset}, ${p.free_managed_max_timeout}s)`,
                included: true,
              },
            ]
          : []),
        ...(p.compute_credit_microusd > 0
          ? [
              {
                name: `${formatComputeCredit(p.compute_credit_microusd)}/mo compute credit`,
                included: true,
              },
            ]
          : []),
        ...(isFree
          ? []
          : [
              {
                name: `${formatLimit(p.max_concurrent_runs)} concurrent runs`,
                included: true,
              },
            ]),
        {
          name: `${formatLimit(p.max_orgs_per_user)} organization${p.max_orgs_per_user === 1 ? "" : "s"}`,
          included: true,
        },
        {
          name: `${formatLimit(p.max_projects_per_org)} project${p.max_projects_per_org === 1 ? "" : "s"}${p.max_projects_per_org === -1 ? "" : " per org"}`,
          included: true,
        },
        {
          name: `${formatLimit(p.max_members_per_org)} member${p.max_members_per_org === 1 ? "" : "s"}${p.max_members_per_org === -1 ? "" : " per org"}`,
          included: true,
        },
        {
          name: `${formatRetention(p.retention_days)} retention`,
          included: true,
        },
        {
          name: `${formatRegionCount(p.allowed_regions)} region${p.allowed_regions.length === 1 ? "" : "s"}`,
          included: true,
        },
        ...(p.has_rbac
          ? [{ name: `${formatRBAC(p.rbac_level)} RBAC`, included: true }]
          : []),
        ...(p.has_audit_logs ? [{ name: "Audit logs", included: true }] : []),
        ...(p.ai_assistant_byok
          ? [{ name: "AI Assistant BYOK", included: true }]
          : []),
        { name: formatSupportLevel(p.support_level), included: true }
      );
    }

    const monthlyInYearly =
      p.price_annual_usd > 0 ? Math.floor(p.price_annual_usd / 12) : undefined;

    return {
      name: p.display_name,
      slug,
      description: PLAN_DESCRIPTIONS[slug] ?? "",
      prices: {
        monthly: p.price_monthly_usd,
        yearly: p.price_annual_usd,
        monthlyInYearly,
      },
      features,
      highlight: slug === "pro",
      badge: isFree ? "No credit card required" : undefined,
      badgeVariant: isFree ? ("success-light" as const) : undefined,
      isCustomPricing: isEnterprise,
    };
  });
};

/** A single row in the plan comparison table. */
export type ComparisonFeature = {
  /** Feature name. */
  name: string;
  /** Value for the free tier. */
  free: string;
  /** Value for the starter tier. */
  starter: string;
  /** Value for the pro tier. */
  pro: string;
  /** Value for the scale tier. */
  scale: string;
  /** Value for the enterprise tier. */
  enterprise: string;
};

/**
 * Transform API plan data into comparison table rows.
 *
 * Each row shows how a feature varies across all five tiers.
 *
 * @param plans - Array of raw API plan data.
 * @returns Array of comparison feature rows for the plan comparison table.
 */
export const apiPlansToComparisonFeatures = (
  plans: APIPlan[]
): ComparisonFeature[] => {
  const byTier = Object.fromEntries(plans.map((p) => [p.tier, p])) as Record<
    string,
    APIPlan
  >;

  const val = (tier: string, fn: (p: APIPlan) => string): string => {
    const p = byTier[tier];
    return p ? fn(p) : "-";
  };

  const row = (
    name: string,
    fn: (p: APIPlan) => string
  ): ComparisonFeature => ({
    name,
    free: val("free", fn),
    starter: val("starter", fn),
    pro: val("pro", fn),
    scale: val("scale", fn),
    enterprise: val("enterprise", fn),
  });

  return [
    row("Runs per day", (p) => formatLimit(p.max_runs_per_day)),
    row("Concurrent runs", (p) => formatLimit(p.max_concurrent_runs)),
    row("Compute credit", (p) =>
      formatComputeCredit(p.compute_credit_microusd)
    ),
    row("Projects", (p) => formatLimit(p.max_projects_per_org)),
    row("Team members", (p) => formatLimit(p.max_members_per_org)),
    row("Retention", (p) => formatRetention(p.retention_days)),
    row("Regions", (p) => formatRegionCount(p.allowed_regions)),
    row("AI model calls/day", (p) => formatLimit(p.max_ai_model_calls_per_day)),
    row("RBAC", (p) => formatRBAC(p.rbac_level)),
    row("Audit logs", (p) => formatBoolean(p.has_audit_logs)),
    row("SSO", (p) => formatBoolean(p.has_sso)),
    row("SLA", (p) => formatBoolean(p.has_sla)),
    row("Webhook subscriptions", (p) =>
      formatLimit(p.max_webhook_subs_per_project)
    ),
    row("Log drains", (p) => formatLimit(p.max_log_drains_per_org)),
    row("Alert rules", (p) => formatLimit(p.max_alert_rules_per_project)),
    row("Dedicated compute", (p) => formatBoolean(p.has_dedicated_compute)),
    row("Static IPs", (p) => formatBoolean(p.has_static_ips)),
    row("VPC peering", (p) => formatBoolean(p.has_vpc_peering)),
    row("SCIM", (p) => formatBoolean(p.has_scim)),
    row("SIEM export", (p) => formatBoolean(p.has_siem_export)),
  ];
};
