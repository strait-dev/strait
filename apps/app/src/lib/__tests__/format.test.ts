import { describe, expect, it } from "vitest";
import { capitalize, formatMicroUsd } from "../format";

describe("formatMicroUsd", () => {
  it("formats zero", () => {
    expect(formatMicroUsd(0)).toBe("$0.00");
  });

  it("formats whole dollars", () => {
    expect(formatMicroUsd(1_000_000)).toBe("$1.00");
  });

  it("formats fractional dollars", () => {
    expect(formatMicroUsd(1_500_000)).toBe("$1.50");
  });

  it("formats sub-cent precision", () => {
    expect(formatMicroUsd(123_456)).toBe("$0.12");
  });

  it("formats large values", () => {
    expect(formatMicroUsd(999_999_999)).toBe("$1000.00");
  });
});

describe("capitalize", () => {
  it("capitalizes lowercase string", () => {
    expect(capitalize("starter")).toBe("Starter");
  });

  it("handles already capitalized string", () => {
    expect(capitalize("Pro")).toBe("Pro");
  });

  it("handles empty string", () => {
    expect(capitalize("")).toBe("");
  });

  it("handles single character", () => {
    expect(capitalize("a")).toBe("A");
  });
});
