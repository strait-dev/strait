import { describe, expect, it } from "vitest";
import {
  type AddonTypeSlug,
  ALL_ADDON_TYPES,
  ALL_PLAN_TIERS,
  type LimitAction,
  type PlanTierSlug,
  REFETCH_5M,
  REFETCH_10M,
  STALE_30S,
} from "../types";

describe("PlanTierSlug", () => {
  it("ALL_PLAN_TIERS contains all 6 tiers in order", () => {
    expect(ALL_PLAN_TIERS).toEqual([
      "free",
      "starter",
      "pro",
      "scale",
      "business",
      "enterprise",
    ]);
  });

  it("type accepts valid tier strings", () => {
    const tiers: PlanTierSlug[] = [
      "free",
      "starter",
      "pro",
      "scale",
      "business",
      "enterprise",
    ];
    expect(tiers).toHaveLength(6);
  });
});

describe("AddonTypeSlug", () => {
  it("ALL_ADDON_TYPES contains all 5 addon types", () => {
    expect(ALL_ADDON_TYPES).toEqual([
      "concurrent_runs",
      "members",
      "cron_schedules",
      "data_retention",
      "webhook_endpoints",
    ]);
  });

  it("type accepts valid addon type strings", () => {
    const types: AddonTypeSlug[] = [
      "concurrent_runs",
      "members",
      "cron_schedules",
      "data_retention",
      "webhook_endpoints",
    ];
    expect(types).toHaveLength(5);
  });
});

describe("LimitAction", () => {
  it("accepts 'reject' and 'notify'", () => {
    const actions: LimitAction[] = ["reject", "notify"];
    expect(actions).toHaveLength(2);
  });
});

describe("refetch constants", () => {
  it("REFETCH_5M is 300,000 ms", () => {
    expect(REFETCH_5M).toBe(300_000);
  });

  it("REFETCH_10M is 600,000 ms", () => {
    expect(REFETCH_10M).toBe(600_000);
  });

  it("STALE_30S is 30,000 ms", () => {
    expect(STALE_30S).toBe(30_000);
  });
});
