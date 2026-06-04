/**
 * Plan definitions and formatting utilities.
 *
 * Exposes plan tier data and transforms it into UI-friendly structures for the pricing page,
 * plan comparison table, and upgrade flow.
 */

import {
  PLAN_API_RESPONSE,
  type PlanApiResponse,
} from "@strait/billing/products";
import { createServerFn } from "@tanstack/react-start";

export {
  formatBoolean,
  formatCronInterval,
  formatLimit,
  formatMicroUsdPrice,
  formatRBAC,
  formatRetention,
  formatSupportLevel,
} from "./plan-formatters";

import {
  formatBoolean,
  formatCronInterval,
  formatLimit,
  formatMicroUsdPrice,
  formatRBAC,
  formatRetention,
  formatSupportLevel,
} from "./plan-formatters";

/** Raw plan data for upgrade and billing screens. */
export type APIPlan = PlanApiResponse;

/** API response wrapper for the plans endpoint. */
export type PlansResponse = {
  plans: APIPlan[];
};

const PLANS_RESPONSE: PlansResponse = {
  plans: PLAN_API_RESPONSE,
};

/**
 * Server function to fetch all plan tier definitions.
 *
 * @returns Array of plan definitions ordered by tier rank.
 */
export const getPlansServerFn = createServerFn({ method: "GET" }).handler(
  () => PLANS_RESPONSE.plans
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
  slug: "free" | "starter" | "pro" | "scale" | "business" | "enterprise";
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
  free: "For evaluation, demos, and side projects.",
  starter: "For solo developers, small projects, and internal tools.",
  pro: "For production SaaS and mid-market teams.",
  scale: "For high-volume teams that need audit logs and canary deploys.",
  business:
    "For enterprise volume and regulated workloads before a custom contract.",
  enterprise: "For custom contracts and roadmap enterprise deployment needs.",
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
      { name: "Core orchestration", included: true },
    ];

    if (isEnterprise) {
      features.push(
        { name: "50M+ orchestration runs/mo", included: true },
        { name: "Custom concurrency and retention", included: true },
        { name: "Multi-org consolidated invoicing", included: true },
        { name: "99.99% SLA target (non-contractual)", included: true },
        {
          name: `${formatRetention(p.retention_days)} retention`,
          included: true,
        },
        { name: formatSupportLevel(p.support_level), included: true },
        {
          name: "Roadmap security features are not launch entitlements",
          included: false,
        }
      );
    } else {
      features.push(
        {
          name: `${formatLimit(p.max_runs_per_month)} orchestration runs/mo`,
          included: true,
        },
        ...(p.overage_per_k_runs_microusd > 0
          ? [
              {
                name: `${formatMicroUsdPrice(p.overage_per_k_runs_microusd)}/1K orchestration runs overage`,
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
        ...(p.has_rbac
          ? [{ name: `${formatRBAC(p.rbac_level)} RBAC`, included: true }]
          : []),
        ...(p.has_audit_logs ? [{ name: "Audit logs", included: true }] : []),
        ...(p.has_log_streaming
          ? [{ name: "Log streaming", included: true }]
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
  /** Value for the business tier. */
  business: string;
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
    business: val("business", fn),
    enterprise: val("enterprise", fn),
  });

  const roadmapRow = (name: string, feature: string): ComparisonFeature =>
    row(name, (p) =>
      p.roadmap_features.includes(feature) ? "Roadmap" : formatBoolean(false)
    );

  return [
    row("Runs per month", (p) => formatLimit(p.max_runs_per_month)),
    row("Overage per 1K runs", (p) =>
      formatMicroUsdPrice(p.overage_per_k_runs_microusd)
    ),
    row("Concurrent runs", (p) => formatLimit(p.max_concurrent_runs)),
    row("Workflow steps", (p) => formatLimit(p.max_workflow_dag_steps)),
    row("Projects", (p) => formatLimit(p.max_projects_per_org)),
    row("Active environments", (p) => formatLimit(p.max_environments)),
    row("Team members", (p) => formatLimit(p.max_members_per_org)),
    row("Cron schedules", (p) => formatLimit(p.max_scheduled_jobs)),
    row("Cron minimum interval", (p) =>
      formatCronInterval(p.cron_min_interval_sec)
    ),
    row("Retention", (p) => formatRetention(p.retention_days)),
    row("Worker connections", (p) => formatLimit(p.worker_connections)),
    row("API rate limit", (p) =>
      p.api_rate_limit === -1
        ? "Unlimited"
        : `${formatLimit(p.api_rate_limit)}/min`
    ),
    row("RBAC", (p) => formatRBAC(p.rbac_level)),
    row("Audit logs", (p) => formatBoolean(p.has_audit_logs)),
    row("SLA target", (p) => formatBoolean(p.has_sla)),
    row("Log streaming", (p) => formatBoolean(p.has_log_streaming)),
    row("Webhook endpoints", (p) => formatLimit(p.max_webhook_endpoints)),
    row("Log drains", (p) => formatLimit(p.max_log_drains_per_org)),
    roadmapRow("SSO/SAML", "SSO/SAML"),
    roadmapRow("SCIM", "SCIM"),
    roadmapRow("IP allowlisting", "IP allowlisting"),
    roadmapRow("Static IPs", "static IPs"),
    roadmapRow("VPC peering", "VPC peering"),
    roadmapRow("Data residency", "data residency"),
    row("Single-tenant / BYO-cloud", (p) =>
      p.roadmap_features.some((feature) =>
        ["single-tenant orchestration", "BYO-cloud"].includes(feature)
      )
        ? "Contact sales"
        : formatBoolean(false)
    ),
  ];
};
