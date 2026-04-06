import { describe, expect, it } from "vitest";

import {
  AGENT_SPENDING_PRESETS,
  computeCreditPercent,
  formatAgentPlanTier,
  formatMicroUsd,
  formatTokenCount,
  usdToMicrousd,
} from "../agent-billing-utils";

describe("formatMicroUsd", () => {
  it("formats positive values to USD", () => {
    expect(formatMicroUsd(5_000_000)).toBe("$5.00");
    expect(formatMicroUsd(39_000_000)).toBe("$39.00");
    expect(formatMicroUsd(149_000_000)).toBe("$149.00");
    expect(formatMicroUsd(1500)).toBe("$0.00");
  });

  it("returns dash for negative values (disabled)", () => {
    expect(formatMicroUsd(-1)).toBe("-");
    expect(formatMicroUsd(-1_000_000)).toBe("-");
  });

  it("handles zero", () => {
    expect(formatMicroUsd(0)).toBe("$0.00");
  });
});

describe("formatTokenCount", () => {
  it("formats millions with M suffix", () => {
    expect(formatTokenCount(1_500_000)).toBe("1.5M");
    expect(formatTokenCount(10_000_000)).toBe("10.0M");
  });

  it("formats thousands with K suffix", () => {
    expect(formatTokenCount(15_000)).toBe("15.0K");
    expect(formatTokenCount(1500)).toBe("1.5K");
  });

  it("formats small numbers with locale string", () => {
    expect(formatTokenCount(500)).toBe("500");
    expect(formatTokenCount(0)).toBe("0");
  });
});

describe("formatAgentPlanTier", () => {
  it("maps known tiers to display names", () => {
    expect(formatAgentPlanTier("agent_free")).toBe("Free");
    expect(formatAgentPlanTier("agent_maker")).toBe("Maker");
    expect(formatAgentPlanTier("agent_growth")).toBe("Growth");
    expect(formatAgentPlanTier("agent_enterprise")).toBe("Enterprise");
  });

  it("returns raw value for unknown tiers", () => {
    expect(formatAgentPlanTier("unknown_tier")).toBe("unknown_tier");
    expect(formatAgentPlanTier("")).toBe("");
  });
});

describe("computeCreditPercent", () => {
  it("computes percentage within budget", () => {
    expect(computeCreditPercent(25, 39)).toBeCloseTo(64.1, 0);
  });

  it("computes percentage in overage", () => {
    expect(computeCreditPercent(55, 39)).toBeGreaterThan(100);
  });

  it("returns 0 for zero included credit", () => {
    expect(computeCreditPercent(10, 0)).toBe(0);
  });

  it("returns 0 for negative included credit", () => {
    expect(computeCreditPercent(10, -1)).toBe(0);
  });
});

describe("usdToMicrousd", () => {
  it("converts whole dollars", () => {
    expect(usdToMicrousd(39)).toBe(39_000_000);
    expect(usdToMicrousd(149)).toBe(149_000_000);
  });

  it("converts fractional dollars", () => {
    expect(usdToMicrousd(0.5)).toBe(500_000);
    expect(usdToMicrousd(25.99)).toBe(25_990_000);
  });

  it("handles zero", () => {
    expect(usdToMicrousd(0)).toBe(0);
  });
});

describe("AGENT_SPENDING_PRESETS", () => {
  it("contains expected preset values", () => {
    expect(AGENT_SPENDING_PRESETS).toEqual([25, 50, 100, 250, 500]);
  });

  it("all presets are positive integers", () => {
    for (const preset of AGENT_SPENDING_PRESETS) {
      expect(preset).toBeGreaterThan(0);
      expect(Number.isInteger(preset)).toBe(true);
    }
  });
});
