import { describe, expect, it } from "vitest";
import {
  formatBoolean,
  formatComputeCredit,
  formatLimit,
  formatRBAC,
  formatRegionCount,
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

describe("formatComputeCredit", () => {
  it("returns '-' for 0", () => {
    expect(formatComputeCredit(0)).toBe("-");
  });

  it("returns '-' for negative values", () => {
    expect(formatComputeCredit(-1)).toBe("-");
  });

  it("formats micro-USD to dollars with 2 decimals", () => {
    expect(formatComputeCredit(1_000_000)).toBe("$1.00");
    expect(formatComputeCredit(19_990_000)).toBe("$19.99");
    expect(formatComputeCredit(99_000_000)).toBe("$99.00");
  });
});

describe("formatRegionCount", () => {
  it("returns 'All' for null regions", () => {
    expect(formatRegionCount(null)).toBe("All");
  });

  it("returns 'All' for empty array", () => {
    expect(formatRegionCount([])).toBe("All");
  });

  it("returns count as string for non-empty array", () => {
    expect(formatRegionCount(["iad"])).toBe("1");
    expect(formatRegionCount(["iad", "lhr", "fra"])).toBe("3");
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
  compute_credit_microusd: 499_000_000,
  retention_days: 90,
  allowed_regions: null,
  max_webhook_subs_per_project: -1,
  max_log_drains_per_org: -1,
  has_rbac: true,
  rbac_level: "advanced",
  has_audit_logs: true,
  has_canary_deployments: true,
  has_approval_gates: true,
  has_sub_workflows: true,
  has_job_chaining: true,
  has_compensating_txns: true,
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

describe("apiPlansToPricingPlans", () => {
  it("models business plans and monthly run caps", () => {
    const [pricingPlan] = apiPlansToPricingPlans([basePlan]);

    expect(pricingPlan.slug).toBe("business");
    expect(pricingPlan.description).not.toBe("");
    expect(pricingPlan.features.map((feature) => feature.name)).toContain(
      "25,000,000 orchestration runs/mo"
    );
    expect(pricingPlan.features.map((feature) => feature.name)).not.toContain(
      "All regions"
    );
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
});
