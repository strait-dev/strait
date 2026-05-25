import { describe, expect, it } from "vitest";
import type { SingletonOnConflict } from "@/hooks/api/types";
import {
  isSingletonConfigured,
  SINGLETON_CONFLICT_LABELS,
  singletonKeyTemplate,
} from "../singleton";

describe("singletonKeyTemplate", () => {
  it("returns a bare string expression unchanged", () => {
    expect(singletonKeyTemplate("tenant-key")).toBe("tenant-key");
  });

  it("extracts the template from an object envelope", () => {
    expect(singletonKeyTemplate({ template: "tenant-key" })).toBe("tenant-key");
  });

  it("returns an empty string for an envelope without a template", () => {
    expect(singletonKeyTemplate({})).toBe("");
  });

  it("returns an empty string when the template is not a string", () => {
    expect(singletonKeyTemplate({ template: 42 })).toBe("");
  });

  it("returns an empty string for null, undefined, and other garbage", () => {
    expect(singletonKeyTemplate(null)).toBe("");
    expect(singletonKeyTemplate(undefined)).toBe("");
    expect(singletonKeyTemplate(123)).toBe("");
    expect(singletonKeyTemplate(["a"])).toBe("");
  });
});

describe("isSingletonConfigured", () => {
  it("is true when an on-conflict policy is set", () => {
    expect(isSingletonConfigured({ singleton_on_conflict: "queue" })).toBe(
      true
    );
  });

  it("is false when the policy is unset, empty, or null", () => {
    expect(isSingletonConfigured({})).toBe(false);
    expect(isSingletonConfigured({ singleton_on_conflict: "" })).toBe(false);
    expect(isSingletonConfigured({ singleton_on_conflict: null })).toBe(false);
  });
});

describe("SINGLETON_CONFLICT_LABELS", () => {
  it("has a label for every on-conflict policy", () => {
    const policies: SingletonOnConflict[] = ["queue", "drop", "replace"];
    for (const policy of policies) {
      expect(SINGLETON_CONFLICT_LABELS[policy]).toBeTruthy();
    }
  });

  it("maps each policy to its human-readable label", () => {
    expect(SINGLETON_CONFLICT_LABELS).toEqual({
      queue: "Queue",
      drop: "Drop",
      replace: "Replace",
    });
  });
});
