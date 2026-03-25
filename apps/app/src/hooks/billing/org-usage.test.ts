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
});
