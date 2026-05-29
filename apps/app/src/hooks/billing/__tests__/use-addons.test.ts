import { describe, expect, it } from "vitest";
import { type AddonTypeSlug, ALL_ADDON_TYPES } from "../types";
import { ADDON_CATALOG, getActivePackCount } from "../use-addons";

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
    for (const addonType of [
      "concurrency_100",
      "history_30d",
      "environments_5",
    ]) {
      expect(catalogTypes).toContain(addonType);
    }
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
    const validTypes: readonly AddonTypeSlug[] = ALL_ADDON_TYPES;
    for (const item of ADDON_CATALOG) {
      expect(validTypes).toContain(item.type);
    }
  });
});
