import { describe, expect, it } from "vitest";
import { isDowngrade } from "../plan-tiers";

describe("isDowngrade", () => {
  it("returns true when going from pro to free", () => {
    expect(isDowngrade("pro", "free")).toBe(true);
  });

  it("returns true when going from pro to starter", () => {
    expect(isDowngrade("pro", "starter")).toBe(true);
  });

  it("returns true when going from enterprise to pro", () => {
    expect(isDowngrade("enterprise", "pro")).toBe(true);
  });

  it("returns true when going from enterprise to free", () => {
    expect(isDowngrade("enterprise", "free")).toBe(true);
  });

  it("returns true when going from starter to free", () => {
    expect(isDowngrade("starter", "free")).toBe(true);
  });

  it("returns true when going from scale to pro", () => {
    expect(isDowngrade("scale", "pro")).toBe(true);
  });

  it("returns true when going from scale to starter", () => {
    expect(isDowngrade("scale", "starter")).toBe(true);
  });

  it("returns true when going from scale to free", () => {
    expect(isDowngrade("scale", "free")).toBe(true);
  });

  it("returns true when going from enterprise to scale", () => {
    expect(isDowngrade("enterprise", "scale")).toBe(true);
  });

  it("returns false when going from free to starter (upgrade)", () => {
    expect(isDowngrade("free", "starter")).toBe(false);
  });

  it("returns false when going from starter to pro (upgrade)", () => {
    expect(isDowngrade("starter", "pro")).toBe(false);
  });

  it("returns false when going from pro to scale (upgrade)", () => {
    expect(isDowngrade("pro", "scale")).toBe(false);
  });

  it("returns false when going from scale to enterprise (upgrade)", () => {
    expect(isDowngrade("scale", "enterprise")).toBe(false);
  });

  it("returns false when going from free to enterprise (upgrade)", () => {
    expect(isDowngrade("free", "enterprise")).toBe(false);
  });

  it("returns false when going from free to scale (upgrade)", () => {
    expect(isDowngrade("free", "scale")).toBe(false);
  });

  it("returns false for same plan", () => {
    expect(isDowngrade("pro", "pro")).toBe(false);
  });

  it("returns false for same plan (free)", () => {
    expect(isDowngrade("free", "free")).toBe(false);
  });

  it("returns false for same plan (scale)", () => {
    expect(isDowngrade("scale", "scale")).toBe(false);
  });

  it("returns false when currentTier is undefined", () => {
    expect(isDowngrade(undefined, "pro")).toBe(false);
  });

  it("returns false when targetTier is undefined", () => {
    expect(isDowngrade("pro", undefined)).toBe(false);
  });

  it("returns false when both are undefined", () => {
    expect(isDowngrade(undefined, undefined)).toBe(false);
  });

  it("treats unknown tiers as rank 0 (same as free)", () => {
    expect(isDowngrade("unknown", "free")).toBe(false);
    expect(isDowngrade("pro", "unknown")).toBe(true);
  });
});
