import { describe, expect, it } from "vitest";
import { periodToDateRange } from "../period-utils";

describe("periodToDateRange", () => {
  it("converts January correctly", () => {
    const result = periodToDateRange("2026-01");
    expect(result).toEqual({ from: "2026-01-01", to: "2026-01-31" });
  });

  it("converts February non-leap year correctly", () => {
    const result = periodToDateRange("2026-02");
    expect(result).toEqual({ from: "2026-02-01", to: "2026-02-28" });
  });

  it("converts February leap year correctly", () => {
    const result = periodToDateRange("2028-02");
    expect(result).toEqual({ from: "2028-02-01", to: "2028-02-29" });
  });

  it("converts December correctly", () => {
    const result = periodToDateRange("2026-12");
    expect(result).toEqual({ from: "2026-12-01", to: "2026-12-31" });
  });

  it("converts April (30 days) correctly", () => {
    const result = periodToDateRange("2026-04");
    expect(result).toEqual({ from: "2026-04-01", to: "2026-04-30" });
  });

  it("pads single-digit months", () => {
    const result = periodToDateRange("2026-3");
    expect(result).toEqual({ from: "2026-03-01", to: "2026-03-31" });
  });
});
