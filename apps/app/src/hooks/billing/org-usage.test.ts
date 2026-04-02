import { describe, expect, it } from "vitest";
import {
  EMPTY_ORG_USAGE,
  normalizeOrgUsageData,
  type RawOrgUsageData,
} from "./org-usage";

function baseUsage(): RawOrgUsageData {
  return {
    org_id: "org-1",
    plan: "starter",
    period: { start: "2026-03-01", end: "2026-03-31" },
    included_credit_microusd: 19_990_000,
    period_spend_microusd: 0,
    overage_microusd: 0,
    credit_used_percent: 0,
    credit_remaining_microusd: 19_990_000,
    usage: {
      runs_today: { used: 10, limit: 100, percent: 10 },
      concurrent_runs: { used: 1, limit: 5, percent: 20 },
      compute_credit: { used: 0, limit: 1_000_000, percent: 0 },
      projects: { used: 1, limit: 5, percent: 20 },
      members: { used: 2, limit: 10, percent: 20 },
      retention_days: 7,
      regions_available: 6,
    },
    alerts: [],
  };
}

describe("normalizeOrgUsageData", () => {
  it("prefers ai_model_calls_today when present", () => {
    const data = normalizeOrgUsageData({
      ...baseUsage(),
      usage: {
        ...baseUsage().usage,
        ai_model_calls_today: { used: 4, limit: 100, percent: 4 },
        ai_assistant_messages_today: { used: 9, limit: 100, percent: 9 },
      },
    });

    expect(data.usage.ai_model_calls_today.used).toBe(4);
    expect(data.usage.ai_assistant_messages_today.used).toBe(9);
  });

  it("falls back to the deprecated ai_assistant_messages_today field", () => {
    const data = normalizeOrgUsageData({
      ...baseUsage(),
      usage: {
        ...baseUsage().usage,
        ai_assistant_messages_today: { used: 7, limit: 100, percent: 7 },
      },
    });

    expect(data.usage.ai_model_calls_today.used).toBe(7);
    expect(data.usage.ai_assistant_messages_today.used).toBe(7);
  });

  it("falls back to the empty default when neither AI field is present", () => {
    const data = normalizeOrgUsageData(baseUsage());

    expect(data.usage.ai_model_calls_today).toEqual(
      EMPTY_ORG_USAGE.usage.ai_model_calls_today
    );
    expect(data.usage.ai_assistant_messages_today).toEqual(
      EMPTY_ORG_USAGE.usage.ai_assistant_messages_today
    );
  });

  it("preserves enterprise fields when present", () => {
    const data = normalizeOrgUsageData({
      ...baseUsage(),
      plan: "enterprise",
      enterprise_tier: "enterprise_starter",
      contract_end_date: "2027-03-31",
      compute_discount_pct: 10,
      sla_uptime_pct: 99.9,
    });

    expect(data.enterprise_tier).toBe("enterprise_starter");
    expect(data.contract_end_date).toBe("2027-03-31");
    expect(data.compute_discount_pct).toBe(10);
    expect(data.sla_uptime_pct).toBe(99.9);
  });

  it("omits enterprise fields when absent", () => {
    const data = normalizeOrgUsageData(baseUsage());

    expect(data.enterprise_tier).toBeUndefined();
    expect(data.contract_end_date).toBeUndefined();
    expect(data.compute_discount_pct).toBeUndefined();
    expect(data.sla_uptime_pct).toBeUndefined();
  });
});

describe("EMPTY_ORG_USAGE", () => {
  it("has plan set to 'free'", () => {
    expect(EMPTY_ORG_USAGE.plan).toBe("free");
  });

  it("has empty org_id", () => {
    expect(EMPTY_ORG_USAGE.org_id).toBe("");
  });

  it("has 1-day retention", () => {
    expect(EMPTY_ORG_USAGE.usage.retention_days).toBe(1);
  });

  it("has 1 available region", () => {
    expect(EMPTY_ORG_USAGE.usage.regions_available).toBe(1);
  });

  it("has empty alerts array", () => {
    expect(EMPTY_ORG_USAGE.alerts).toEqual([]);
  });

  it("has zero credit values", () => {
    expect(EMPTY_ORG_USAGE.included_credit_microusd).toBe(0);
    expect(EMPTY_ORG_USAGE.period_spend_microusd).toBe(0);
    expect(EMPTY_ORG_USAGE.overage_microusd).toBe(0);
    expect(EMPTY_ORG_USAGE.credit_used_percent).toBe(0);
    expect(EMPTY_ORG_USAGE.credit_remaining_microusd).toBe(0);
  });
});
