import { describe, expect, it } from "vitest";
import type { SingletonHolder, SingletonOnConflict } from "@/hooks/api/types";
import {
  findSingletonHolderForKey,
  isSingletonConfigured,
  SINGLETON_CONFLICT_LABELS,
  singletonKeyTemplate,
} from "../singleton";

const holder = (lock_key: string): SingletonHolder => ({
  lock_key,
  holder_run_id: `run-${lock_key}`,
  acquired_at: "2026-01-01T00:00:00Z",
  waiters: 0,
});

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

describe("findSingletonHolderForKey", () => {
  const holders = [holder("tenant-a"), holder("tenant-b")];

  it("returns the holder whose lock key matches", () => {
    expect(findSingletonHolderForKey(holders, "tenant-b")).toBe(holders[1]);
  });

  it("returns undefined when no holder matches the key", () => {
    expect(findSingletonHolderForKey(holders, "tenant-c")).toBeUndefined();
  });

  it("returns undefined for a missing key or holder list", () => {
    expect(findSingletonHolderForKey(holders, undefined)).toBeUndefined();
    expect(findSingletonHolderForKey(holders, "")).toBeUndefined();
    expect(findSingletonHolderForKey(undefined, "tenant-a")).toBeUndefined();
  });
});
