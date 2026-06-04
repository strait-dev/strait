import {
  ACTIVE_ADDON_KEYS,
  ROADMAP_ADDON_KEYS,
} from "@strait/billing/products";
import { Either, Schema } from "effect";
import { describe, expect, it } from "vitest";
import {
  OrgUsageResponseSchema,
  SpendingLimitSchema,
  UsageForecastSchema,
} from "../schemas";

const validOrgUsage = {
  org_id: "org-1",
  plan: "starter",
  period: { start: "2026-03-01", end: "2026-03-31" },
  usage: {
    monthly_runs: { used: 10, limit: 50_000, percent: 0.02 },
    runs_today: { used: 10, limit: 100, percent: 10 },
    concurrent_runs: { used: 1, limit: 5, percent: 20 },
    projects: { used: 1, limit: 5, percent: 20 },
    members: { used: 2, limit: 10, percent: 20 },
    retention_days: 7,
  },
  period_spend_microusd: 0,
  overage_microusd: 0,
  alerts: [],
};

describe("OrgUsageResponseSchema", () => {
  const decode = Schema.decodeUnknownEither(OrgUsageResponseSchema);

  it("decodes a valid payload successfully", () => {
    const result = decode(validOrgUsage);
    expect(Either.isRight(result)).toBe(true);
    if (Either.isRight(result)) {
      expect(result.right.org_id).toBe("org-1");
      expect(result.right.plan).toBe("starter");
    }
  });

  it("fails when org_id is missing", () => {
    const { org_id: _, ...missing } = validOrgUsage;
    const result = decode(missing);
    expect(Either.isLeft(result)).toBe(true);
  });

  it("fails when period_spend_microusd is wrong type", () => {
    const result = decode({
      ...validOrgUsage,
      period_spend_microusd: "not a number",
    });
    expect(Either.isLeft(result)).toBe(true);
  });

  it("decodes enterprise fields when present", () => {
    const result = decode({
      ...validOrgUsage,
      enterprise_tier: "enterprise_starter",
      contract_end_date: "2027-03-31",
      overage_discount_pct: 10,
      sla_uptime_pct: 99.9,
    });
    expect(Either.isRight(result)).toBe(true);
    if (Either.isRight(result)) {
      expect(result.right.enterprise_tier).toBe("enterprise_starter");
      expect(result.right.overage_discount_pct).toBe(10);
    }
  });

  it("succeeds when enterprise fields are absent", () => {
    const result = decode(validOrgUsage);
    expect(Either.isRight(result)).toBe(true);
    if (Either.isRight(result)) {
      expect(result.right.enterprise_tier).toBeUndefined();
    }
  });

  it("decodes alerts array", () => {
    const result = decode({
      ...validOrgUsage,
      alerts: [
        {
          type: "approaching_limit",
          dimension: "monthly_runs",
          threshold: 80,
          message: "You've used 80% of monthly runs",
        },
      ],
    });
    expect(Either.isRight(result)).toBe(true);
    if (Either.isRight(result)) {
      expect(result.right.alerts).toHaveLength(1);
      expect(result.right.alerts[0].type).toBe("approaching_limit");
    }
  });

  it("decodes every launch-active addon type when present", () => {
    for (const addonType of ACTIVE_ADDON_KEYS) {
      const result = decode({
        ...validOrgUsage,
        active_addons: [{ type: addonType, quantity: 2 }],
      });
      expect(Either.isRight(result)).toBe(true);
      if (Either.isRight(result)) {
        expect(result.right.active_addons).toHaveLength(1);
        expect(result.right.active_addons?.[0]?.type).toBe(addonType);
      }
    }
  });

  it("rejects legacy addon types in active_addons", () => {
    const result = decode({
      ...validOrgUsage,
      active_addons: [{ type: "members", quantity: 2 }],
    });
    expect(Either.isLeft(result)).toBe(true);
  });

  it("rejects every roadmap-only addon type in active_addons", () => {
    for (const addonType of ROADMAP_ADDON_KEYS) {
      const result = decode({
        ...validOrgUsage,
        active_addons: [{ type: addonType, quantity: 1 }],
      });
      expect(Either.isLeft(result)).toBe(true);
    }
  });
});

describe("SpendingLimitSchema", () => {
  const decode = Schema.decodeUnknownEither(SpendingLimitSchema);

  it("decodes a valid payload", () => {
    const result = decode({
      org_id: "org-1",
      plan_tier: "pro",
      overage_enabled: true,
      spending_limit_usd: 500,
      limit_action: "reject",
      current_spend_usd: 100,
      overage_spend_usd: 50.01,
      is_hard_capped: false,
    });
    expect(Either.isRight(result)).toBe(true);
  });

  it("fails when is_hard_capped is wrong type", () => {
    const result = decode({
      org_id: "org-1",
      plan_tier: "pro",
      overage_enabled: true,
      spending_limit_usd: 500,
      limit_action: "reject",
      current_spend_usd: 100,
      overage_spend_usd: 50.01,
      is_hard_capped: "yes",
    });
    expect(Either.isLeft(result)).toBe(true);
  });
});

describe("UsageForecastSchema", () => {
  const decode = Schema.decodeUnknownEither(UsageForecastSchema);

  it("decodes a valid payload", () => {
    const result = decode({
      projected_monthly_runs: 50_000,
      projected_monthly_spend_usd: 25.5,
      recommended_plan: "pro",
      days_until_limit: 15,
      projected_overage_microusd: 5_000_000,
      addon_spend_microusd: 100_000,
      scale_breakeven: false,
    });
    expect(Either.isRight(result)).toBe(true);
  });

  it("fails when required field is missing", () => {
    const result = decode({
      projected_monthly_runs: 50_000,
      // missing other fields
    });
    expect(Either.isLeft(result)).toBe(true);
  });
});
