import { describe, expect, it } from "vitest";
import {
  formatBoolean,
  formatCronInterval,
  formatLimit,
  formatMicroUsdPrice,
  formatRBAC,
  formatRetention,
  formatSupportLevel,
} from "../plan-formatters";
import {
  type APIPlan,
  apiPlansToComparisonFeatures,
  apiPlansToPricingPlans,
} from "../use-plans";

describe("formatLimit", () => {
  it("returns 'Unlimited' for -1", () => {
    expect(formatLimit(-1)).toBe("Unlimited");
  });

  it("returns '0' for 0", () => {
    expect(formatLimit(0)).toBe("0");
  });

  it("returns raw string for values under 1000", () => {
    expect(formatLimit(5)).toBe("5");
    expect(formatLimit(999)).toBe("999");
  });

  it("formats values >= 1000 with locale separators", () => {
    expect(formatLimit(1000)).toBe("1,000");
    expect(formatLimit(100_000)).toBe("100,000");
    expect(formatLimit(1_000_000)).toBe("1,000,000");
  });
});

describe("formatMicroUsdPrice", () => {
  it("returns '-' for 0", () => {
    expect(formatMicroUsdPrice(0)).toBe("-");
  });

  it("returns '-' for negative values", () => {
    expect(formatMicroUsdPrice(-1)).toBe("-");
  });

  it("formats micro-USD to dollars with 2 decimals", () => {
    expect(formatMicroUsdPrice(1_000_000)).toBe("$1.00");
    expect(formatMicroUsdPrice(19_990_000)).toBe("$19.99");
    expect(formatMicroUsdPrice(99_000_000)).toBe("$99.00");
  });
});

describe("formatRetention", () => {
  it("returns '1 day' for singular", () => {
    expect(formatRetention(1)).toBe("1 day");
  });

  it("returns plural for > 1", () => {
    expect(formatRetention(7)).toBe("7 days");
    expect(formatRetention(30)).toBe("30 days");
    expect(formatRetention(90)).toBe("90 days");
  });
});

describe("formatCronInterval", () => {
  it("formats sub-second cron intervals", () => {
    expect(formatCronInterval(0)).toBe("sub-second");
  });

  it("formats second cron intervals", () => {
    expect(formatCronInterval(30)).toBe("30 sec");
  });

  it("formats minute cron intervals", () => {
    expect(formatCronInterval(60)).toBe("1 min");
    expect(formatCronInterval(300)).toBe("5 min");
  });
});

describe("formatRBAC", () => {
  it("returns '-' for empty string", () => {
    expect(formatRBAC("")).toBe("-");
  });

  it("capitalizes the first letter", () => {
    expect(formatRBAC("basic")).toBe("Basic");
    expect(formatRBAC("full")).toBe("Full");
    expect(formatRBAC("advanced")).toBe("Advanced");
  });
});

describe("formatBoolean", () => {
  it("returns 'Yes' for true", () => {
    expect(formatBoolean(true)).toBe("Yes");
  });

  it("returns '-' for false", () => {
    expect(formatBoolean(false)).toBe("-");
  });
});

describe("formatSupportLevel", () => {
  it("maps known levels to labels", () => {
    expect(formatSupportLevel("community")).toBe("Community support");
    expect(formatSupportLevel("email_72h")).toBe("Email support (72h)");
    expect(formatSupportLevel("priority_24h")).toBe("Priority support (24h)");
    expect(formatSupportLevel("priority_slack_8h")).toBe(
      "Priority support + Slack (8h)"
    );
    expect(formatSupportLevel("dedicated")).toBe("Dedicated support + CSM");
  });

  it("returns raw level for unknown values", () => {
    expect(formatSupportLevel("unknown")).toBe("unknown");
    expect(formatSupportLevel("")).toBe("");
  });
});

const basePlan = {
  tier: "business",
  display_name: "Business",
  price_monthly_usd: 49_900,
  price_annual_usd: 478_800,
  max_orgs_per_user: -1,
  max_projects_per_org: -1,
  max_members_per_org: -1,
  max_runs_per_month: 25_000_000,
  max_concurrent_runs: 500,
  overage_default_enabled: true,
  default_spending_cap_microusd: 1_500_000_000,
  retention_days: 90,
  max_webhook_subs_per_project: -1,
  max_log_drains_per_org: -1,
  max_notification_channels: -1,
  has_rbac: true,
  rbac_level: "advanced",
  has_audit_logs: true,
  has_canary_deployments: true,
  has_approval_gates: true,
  has_sub_workflows: true,
  has_job_chaining: true,
  has_compensating_txns: true,
  has_log_streaming: true,
  has_sla: true,
  requires_credit_card: true,
  overage_per_k_runs_microusd: 30_000,
  support_level: "priority_slack_8h",
  max_environments: -1,
  max_scheduled_jobs: -1,
  cron_min_interval_sec: 0,
  max_webhook_endpoints: -1,
  max_workflow_dag_steps: -1,
  api_rate_limit: -1,
  worker_connections: -1,
  roadmap_features: ["SSO/SAML", "SCIM", "IP allowlisting"],
} satisfies APIPlan;

const testPlan = (overrides: Partial<APIPlan>): APIPlan => ({
  ...basePlan,
  ...overrides,
});

describe("apiPlansToPricingPlans", () => {
  it("models business plans and monthly run caps", () => {
    const [pricingPlan] = apiPlansToPricingPlans([basePlan]);

    expect(pricingPlan.slug).toBe("business");
    expect(pricingPlan.description).not.toBe("");
    expect(pricingPlan.features.map((feature) => feature.name)).toContain(
      "25,000,000 orchestration runs/mo"
    );
    expect(
      pricingPlan.features.some((feature) =>
        feature.name.toLowerCase().includes("region")
      )
    ).toBe(false);
  });

  it("marks Enterprise roadmap security as excluded from launch entitlements", () => {
    const [pricingPlan] = apiPlansToPricingPlans([
      testPlan({
        tier: "enterprise",
        display_name: "Enterprise",
        roadmap_features: [
          "SSO/SAML",
          "SCIM",
          "IP allowlisting",
          "static IPs",
          "VPC peering",
          "data residency",
          "single-tenant orchestration",
          "BYO-cloud",
        ],
      }),
    ]);

    expect(pricingPlan.features).toContainEqual({
      name: "99.99% SLA target (non-contractual)",
      included: true,
    });
    expect(pricingPlan.features).toContainEqual({
      name: "Roadmap security features are not launch entitlements",
      included: false,
    });
    expect(
      pricingPlan.features.some((feature) =>
        feature.name.includes(
          ["Roadmap security features", "via sales"].join(" ")
        )
      )
    ).toBe(false);
  });
});

describe("apiPlansToComparisonFeatures", () => {
  it("includes the business column and monthly runs row", () => {
    const rows = apiPlansToComparisonFeatures([basePlan]);

    expect(rows[0]).toMatchObject({
      name: "Runs per month",
      business: "25,000,000",
    });
  });

  it("uses the public org-wide webhook endpoint cap in comparison rows", () => {
    const rows = apiPlansToComparisonFeatures([
      testPlan({
        max_webhook_subs_per_project: 3,
        max_webhook_endpoints: 10,
      }),
    ]);

    expect(rows.find((row) => row.name === "Webhook subscriptions")).toBe(
      undefined
    );
    expect(rows.find((row) => row.name === "Webhook endpoints")).toMatchObject({
      business: "10",
    });
  });

  it("keeps generated launch billing metadata on plan responses", () => {
    expect(basePlan.overage_default_enabled).toBe(true);
    expect(basePlan.default_spending_cap_microusd).toBe(1_500_000_000);
    expect(basePlan.max_notification_channels).toBe(-1);
  });

  it("renders launch-active operational limits from plan API fields", () => {
    const rows = apiPlansToComparisonFeatures([
      testPlan({
        tier: "free",
        display_name: "Free",
        max_workflow_dag_steps: 10,
        max_environments: 1,
        max_scheduled_jobs: 1,
        cron_min_interval_sec: 300,
        worker_connections: 1,
        api_rate_limit: 60,
      }),
      testPlan({
        tier: "business",
        display_name: "Business",
        max_workflow_dag_steps: -1,
        max_environments: -1,
        max_scheduled_jobs: -1,
        cron_min_interval_sec: 0,
        worker_connections: -1,
        api_rate_limit: -1,
      }),
    ]);
    const byName = Object.fromEntries(rows.map((row) => [row.name, row]));

    expect(byName["Workflow steps"]).toMatchObject({
      free: "10",
      business: "Unlimited",
    });
    expect(byName["Active environments"]).toMatchObject({
      free: "1",
      business: "Unlimited",
    });
    expect(byName["Cron schedules"]).toMatchObject({
      free: "1",
      business: "Unlimited",
    });
    expect(byName["Cron minimum interval"]).toMatchObject({
      free: "5 min",
      business: "sub-second",
    });
    expect(byName["Worker connections"]).toMatchObject({
      free: "1",
      business: "Unlimited",
    });
    expect(byName["API rate limit"]).toMatchObject({
      free: "60/min",
      business: "Unlimited",
    });
  });

  it("renders log streaming as a launch-active entitlement", () => {
    const rows = apiPlansToComparisonFeatures([
      testPlan({
        tier: "free",
        display_name: "Free",
        has_log_streaming: false,
      }),
      testPlan({
        tier: "starter",
        display_name: "Starter",
        has_log_streaming: true,
      }),
    ]);

    expect(rows.find((row) => row.name === "Log streaming")).toMatchObject({
      free: "-",
      starter: "Yes",
    });
  });

  it("renders launch-inactive enterprise security features as roadmap only", () => {
    const rows = apiPlansToComparisonFeatures([
      testPlan({
        tier: "free",
        display_name: "Free",
        roadmap_features: [],
      }),
      testPlan({
        tier: "business",
        display_name: "Business",
        roadmap_features: [
          "SSO/SAML",
          "SCIM",
          "IP allowlisting",
          "static IPs",
          "VPC peering",
          "data residency",
        ],
      }),
      testPlan({
        tier: "enterprise",
        display_name: "Enterprise",
        roadmap_features: [
          "SSO/SAML",
          "SCIM",
          "IP allowlisting",
          "static IPs",
          "VPC peering",
          "data residency",
          "single-tenant orchestration",
          "BYO-cloud",
        ],
      }),
    ]);
    const byName = Object.fromEntries(rows.map((row) => [row.name, row]));

    expect(byName["SSO/SAML"]).toMatchObject({
      free: "-",
      business: "Roadmap",
      enterprise: "Roadmap",
    });
    expect(byName["IP allowlisting"]).toMatchObject({
      free: "-",
      business: "Roadmap",
      enterprise: "Roadmap",
    });
    expect(byName.SCIM).toMatchObject({
      free: "-",
      business: "Roadmap",
      enterprise: "Roadmap",
    });
    expect(byName["Static IPs"]).toMatchObject({
      free: "-",
      business: "Roadmap",
      enterprise: "Roadmap",
    });
    expect(byName["VPC peering"]).toMatchObject({
      free: "-",
      business: "Roadmap",
      enterprise: "Roadmap",
    });
    expect(byName["Data residency"]).toMatchObject({
      free: "-",
      business: "Roadmap",
      enterprise: "Roadmap",
    });
    expect(byName["Single-tenant / BYO-cloud"]).toMatchObject({
      free: "-",
      business: "-",
      enterprise: "Contact sales",
    });
  });
});
