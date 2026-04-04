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
