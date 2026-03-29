import { createServerFn } from "@tanstack/react-start";
import { apiRequest } from "@/lib/api-client.server";

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
};

type PlansResponse = {
  plans: APIPlan[];
};

export const getPlansServerFn = createServerFn({ method: "GET" }).handler(
  async () => {
    const data = await apiRequest<PlansResponse>("/v1/plans");
    return data.plans;
  }
);

const MICRO_TO_DOLLARS = 1_000_000;

function formatLimit(value: number): string {
  if (value === -1) {
    return "Unlimited";
  }
  if (value >= 1000) {
    return value.toLocaleString("en-US");
  }
  return String(value);
}

function formatComputeCredit(microusd: number): string {
  if (microusd <= 0) {
    return "-";
  }
  return `$${(microusd / MICRO_TO_DOLLARS).toFixed(2)}`;
}

function formatRegionCount(regions: string[]): string {
  if (regions.length === 0) {
    return "All";
  }
  return String(regions.length);
}

function formatRetention(days: number): string {
  if (days === 1) {
    return "1 day";
  }
  return `${days} days`;
}

function formatRBAC(level: string): string {
  if (!level) {
    return "-";
  }
  return level.charAt(0).toUpperCase() + level.slice(1);
}

function formatBoolean(value: boolean): string {
  return value ? "Yes" : "-";
}

export type PricingFeature = {
  name: string;
  description?: string;
  included: boolean;
};

export type PricingPlan = {
  name: string;
  slug: "free" | "starter" | "pro" | "scale" | "enterprise";
  description: string;
  prices: {
    monthly: number;
    yearly: number;
    monthlyInYearly?: number;
  };
  features: PricingFeature[];
  highlight?: boolean;
  badge?: string;
  badgeVariant?: "success-light" | "info-light" | "default";
  isCustomPricing?: boolean;
};

const PLAN_DESCRIPTIONS: Record<string, string> = {
  free: "For side projects and evaluation. All features included.",
  starter: "For growing teams with production workloads.",
  pro: "For production workloads at scale.",
  scale: "For high-volume teams that need audit logs and canary deploys.",
  enterprise: "Custom everything for large organizations.",
};

const SUPPORT_LABELS: Record<string, string> = {
  community: "Community support",
  email_72h: "Email support (72h)",
  priority_24h: "Priority support (24h)",
  priority_slack_8h: "Priority support + Slack (8h)",
  dedicated: "Dedicated support + CSM",
};

function formatSupportLevel(level: string): string {
  return SUPPORT_LABELS[level] ?? level;
}

export function apiPlansToPricingPlans(plans: APIPlan[]): PricingPlan[] {
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
      highlight: slug === "starter",
      badge: isFree ? "No credit card required" : undefined,
      badgeVariant: isFree ? ("success-light" as const) : undefined,
      isCustomPricing: isEnterprise,
    };
  });
}

export type ComparisonFeature = {
  name: string;
  free: string;
  starter: string;
  pro: string;
  scale: string;
  enterprise: string;
};

export function apiPlansToComparisonFeatures(
  plans: APIPlan[]
): ComparisonFeature[] {
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
  ];
}
