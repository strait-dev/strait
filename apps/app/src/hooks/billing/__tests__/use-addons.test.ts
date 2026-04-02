import { describe, expect, it } from "vitest";
import { ADDON_CATALOG, getActivePackCount } from "../use-addons";
import { ALL_ADDON_TYPES, type AddonTypeSlug } from "../types";

describe("getActivePackCount", () => {
  it("returns 0 for undefined addons", () => {
    expect(getActivePackCount(undefined, "members")).toBe(0);
  });

  it("returns 0 for empty array", () => {
    expect(getActivePackCount([], "members")).toBe(0);
  });

  it("returns quantity for single matching addon", () => {
    expect(
      getActivePackCount([{ type: "members", quantity: 2 }], "members")
    ).toBe(2);
  });

  it("sums quantities for multiple addons of same type", () => {
    expect(
      getActivePackCount(
        [
          { type: "members", quantity: 1 },
          { type: "members", quantity: 3 },
        ],
        "members"
      )
    ).toBe(4);
  });

  it("returns 0 when no addons match the type", () => {
    expect(
      getActivePackCount(
        [{ type: "concurrent_runs", quantity: 5 }],
        "members"
      )
    ).toBe(0);
  });

  it("ignores addons of different types", () => {
    expect(
      getActivePackCount(
        [
          { type: "concurrent_runs", quantity: 5 },
          { type: "members", quantity: 2 },
          { type: "cron_schedules", quantity: 3 },
        ],
        "members"
      )
    ).toBe(2);
  });
});

describe("ADDON_CATALOG", () => {
  it("has exactly 5 items", () => {
    expect(ADDON_CATALOG).toHaveLength(5);
  });

  it("contains all valid addon types", () => {
    const catalogTypes = ADDON_CATALOG.map((item) => item.type);
    for (const addonType of ALL_ADDON_TYPES) {
      expect(catalogTypes).toContain(addonType);
    }
  });

  it("all items have non-empty required fields", () => {
    for (const item of ADDON_CATALOG) {
      expect(item.name).toBeTruthy();
      expect(item.description).toBeTruthy();
      expect(item.price).toBeTruthy();
      expect(item.checkoutSlug).toBeTruthy();
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
