import { describe, expect, it } from "vitest";
import {
  ACTIVE_ADDON_TYPES,
  type ActiveAddonTypeSlug,
  type AddonTypeSlug,
  ALL_ADDON_TYPES,
  ALL_PLAN_TIERS,
  type LimitAction,
  type PlanTierSlug,
  type RBACLevel,
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
  it("ALL_ADDON_TYPES contains active and roadmap addon types", () => {
    expect(ALL_ADDON_TYPES).toEqual([
      "concurrency_100",
      "history_30d",
      "environments_5",
      "compliance_archive",
      "dedicated_workers",
    ]);
  });

  it("ACTIVE_ADDON_TYPES contains only launch-active addon types", () => {
    expect(ACTIVE_ADDON_TYPES).toEqual([
      "concurrency_100",
      "history_30d",
      "environments_5",
    ]);
  });

  it("type accepts valid addon type strings", () => {
    const types: AddonTypeSlug[] = [
      "concurrency_100",
      "history_30d",
      "environments_5",
      "compliance_archive",
      "dedicated_workers",
    ];
    expect(types).toHaveLength(5);
  });

  it("active addon type excludes roadmap-only addons", () => {
    const types: ActiveAddonTypeSlug[] = [
      "concurrency_100",
      "history_30d",
      "environments_5",
    ];
    expect(types).toHaveLength(3);
  });
});

describe("LimitAction", () => {
  it("accepts 'reject' and 'notify'", () => {
    const actions: LimitAction[] = ["reject", "notify"];
    expect(actions).toHaveLength(2);
  });
});

describe("RBACLevel", () => {
  it("accepts every launch RBAC level", () => {
    const levels: RBACLevel[] = ["none", "basic", "full", "advanced"];
    expect(levels).toHaveLength(4);
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
