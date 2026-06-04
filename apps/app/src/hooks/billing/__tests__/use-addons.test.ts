import { describe, expect, it } from "vitest";
import { ACTIVE_ADDON_TYPES, type ActiveAddonTypeSlug } from "../types";
import {
  ADDON_CATALOG,
  getActivePackCount,
  getAddonCatalogItem,
  getAvailableAddonCatalog,
  isAddonAvailableOnPlan,
} from "../use-addons";

describe("getActivePackCount", () => {
  it("returns 0 for undefined addons", () => {
    expect(getActivePackCount(undefined, "concurrency_100")).toBe(0);
  });

  it("returns 0 for empty array", () => {
    expect(getActivePackCount([], "concurrency_100")).toBe(0);
  });

  it("returns quantity for single matching addon", () => {
    expect(
      getActivePackCount(
        [{ type: "concurrency_100", quantity: 2 }],
        "concurrency_100"
      )
    ).toBe(2);
  });

  it("sums quantities for multiple addons of same type", () => {
    expect(
      getActivePackCount(
        [
          { type: "concurrency_100", quantity: 1 },
          { type: "concurrency_100", quantity: 3 },
        ],
        "concurrency_100"
      )
    ).toBe(4);
  });

  it("returns 0 when no addons match the type", () => {
    expect(
      getActivePackCount(
        [{ type: "history_30d", quantity: 5 }],
        "concurrency_100"
      )
    ).toBe(0);
  });

  it("ignores addons of different types", () => {
    expect(
      getActivePackCount(
        [
          { type: "history_30d", quantity: 5 },
          { type: "concurrency_100", quantity: 2 },
          { type: "environments_5", quantity: 3 },
        ],
        "concurrency_100"
      )
    ).toBe(2);
  });
});

describe("ADDON_CATALOG", () => {
  it("has exactly 3 launch-active items", () => {
    expect(ADDON_CATALOG).toHaveLength(3);
  });

  it("contains only launch-active addon types", () => {
    const catalogTypes = ADDON_CATALOG.map((item) => item.type);
    expect(catalogTypes).toEqual(ACTIVE_ADDON_TYPES);
  });

  it("all items have non-empty required fields", () => {
    for (const item of ADDON_CATALOG) {
      expect(item.name).toBeTruthy();
      expect(item.description).toBeTruthy();
      expect(item.price).toBeTruthy();
      expect(item.availableOn.length).toBeGreaterThan(0);
      expect(item.packSize).toBeGreaterThan(0);
      expect(item.packUnit).toBeTruthy();
    }
  });

  it("types are valid AddonTypeSlug values", () => {
    const validTypes: readonly ActiveAddonTypeSlug[] = ACTIVE_ADDON_TYPES;
    for (const item of ADDON_CATALOG) {
      expect(validTypes).toContain(item.type);
    }
  });

  it("does not resolve roadmap-only add-ons as checkout catalog items", () => {
    expect(getAddonCatalogItem("compliance_archive")).toBeUndefined();
    expect(getAddonCatalogItem("dedicated_workers")).toBeUndefined();
  });

  it("enforces per-plan add-on availability", () => {
    expect(isAddonAvailableOnPlan("concurrency_100", "pro")).toBe(true);
    expect(isAddonAvailableOnPlan("history_30d", "pro")).toBe(false);
    expect(isAddonAvailableOnPlan("history_30d", "scale")).toBe(true);
    expect(isAddonAvailableOnPlan("environments_5", "business")).toBe(false);
    expect(isAddonAvailableOnPlan("environments_5", "scale")).toBe(true);
    expect(isAddonAvailableOnPlan("concurrency_100", "starter")).toBe(false);
  });

  it("returns only add-ons available on the current plan", () => {
    expect(getAvailableAddonCatalog("pro").map((addon) => addon.type)).toEqual([
      "concurrency_100",
      "environments_5",
    ]);
    expect(
      getAvailableAddonCatalog("business").map((addon) => addon.type)
    ).toEqual(["concurrency_100", "history_30d"]);
    expect(getAvailableAddonCatalog("enterprise")).toEqual([]);
  });
});
