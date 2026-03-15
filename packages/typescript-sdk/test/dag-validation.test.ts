import { describe, expect, test } from "bun:test";

import { validateDag } from "../src/authoring/dag-validation";
import { step } from "../src/authoring/steps";
import { DagValidationError } from "../src/errors";

describe("DAG validation (Kahn's algorithm)", () => {
  test("valid linear DAG returns topological order", () => {
    const steps = [
      step.job("a", "job_1"),
      step.job("b", "job_2", { dependsOn: ["a"] }),
      step.job("c", "job_3", { dependsOn: ["b"] }),
    ];

    const sorted = validateDag(steps);
    expect(sorted).toEqual(["a", "b", "c"]);
  });

  test("valid diamond DAG returns valid topological order", () => {
    const steps = [
      step.job("a", "job_1"),
      step.job("b", "job_2", { dependsOn: ["a"] }),
      step.job("c", "job_3", { dependsOn: ["a"] }),
      step.job("d", "job_4", { dependsOn: ["b", "c"] }),
    ];

    const sorted = validateDag(steps);
    expect(sorted[0]).toBe("a");
    expect(sorted[sorted.length - 1]).toBe("d");
    expect(sorted.indexOf("b")).toBeGreaterThan(sorted.indexOf("a"));
    expect(sorted.indexOf("c")).toBeGreaterThan(sorted.indexOf("a"));
    expect(sorted.indexOf("d")).toBeGreaterThan(sorted.indexOf("b"));
    expect(sorted.indexOf("d")).toBeGreaterThan(sorted.indexOf("c"));
  });

  test("detects circular dependency", () => {
    const steps = [
      step.job("a", "job_1", { dependsOn: ["c"] }),
      step.job("b", "job_2", { dependsOn: ["a"] }),
      step.job("c", "job_3", { dependsOn: ["b"] }),
    ];

    expect(() => validateDag(steps)).toThrow(DagValidationError);

    try {
      validateDag(steps);
    } catch (e) {
      const err = e as DagValidationError;
      expect(err.message).toContain("Circular dependency");
      expect(err.cycles).toBeDefined();
      expect(err.cycles!.length).toBeGreaterThan(0);
    }
  });

  test("detects missing ref in dependsOn", () => {
    const steps = [
      step.job("a", "job_1"),
      step.job("b", "job_2", { dependsOn: ["nonexistent"] }),
    ];

    expect(() => validateDag(steps)).toThrow(DagValidationError);

    try {
      validateDag(steps);
    } catch (e) {
      const err = e as DagValidationError;
      expect(err.message).toContain("non-existent");
      expect(err.missingRefs).toContain("nonexistent");
    }
  });

  test("detects duplicate step refs", () => {
    const steps = [
      step.job("a", "job_1"),
      step.job("a", "job_2"),
    ];

    expect(() => validateDag(steps)).toThrow(DagValidationError);

    try {
      validateDag(steps);
    } catch (e) {
      const err = e as DagValidationError;
      expect(err.message).toContain("Duplicate");
      expect(err.duplicateRefs).toContain("a");
    }
  });

  test("empty steps returns empty array", () => {
    const sorted = validateDag([]);
    expect(sorted).toEqual([]);
  });

  test("single step returns single-element array", () => {
    const sorted = validateDag([step.job("only", "job_1")]);
    expect(sorted).toEqual(["only"]);
  });

  test("complex multi-path DAG validates correctly", () => {
    const steps = [
      step.job("start", "job_start"),
      step.job("path-a1", "job_a1", { dependsOn: ["start"] }),
      step.job("path-a2", "job_a2", { dependsOn: ["path-a1"] }),
      step.job("path-b1", "job_b1", { dependsOn: ["start"] }),
      step.job("path-b2", "job_b2", { dependsOn: ["path-b1"] }),
      step.job("merge", "job_merge", { dependsOn: ["path-a2", "path-b2"] }),
      step.job("end", "job_end", { dependsOn: ["merge"] }),
    ];

    const sorted = validateDag(steps);
    expect(sorted.length).toBe(7);
    expect(sorted[0]).toBe("start");
    expect(sorted[sorted.length - 1]).toBe("end");
    expect(sorted.indexOf("merge")).toBeGreaterThan(
      sorted.indexOf("path-a2")
    );
    expect(sorted.indexOf("merge")).toBeGreaterThan(
      sorted.indexOf("path-b2")
    );
  });

  test("multiple root nodes (no dependencies) are valid", () => {
    const steps = [
      step.job("root-a", "job_a"),
      step.job("root-b", "job_b"),
      step.job("join", "job_join", { dependsOn: ["root-a", "root-b"] }),
    ];

    const sorted = validateDag(steps);
    expect(sorted.length).toBe(3);
    expect(sorted[sorted.length - 1]).toBe("join");
  });

  test("works with all step types, not just job steps", () => {
    const steps = [
      step.job("validate", "job_validate"),
      step.approval("review", { dependsOn: ["validate"] }),
      step.waitForEvent("confirm", "order.confirmed", { dependsOn: ["review"] }),
      step.sleep("cooldown", 30, { dependsOn: ["confirm"] }),
      step.subWorkflow("notify", "wf_notify", { dependsOn: ["cooldown"] }),
    ];

    const sorted = validateDag(steps);
    expect(sorted).toEqual([
      "validate",
      "review",
      "confirm",
      "cooldown",
      "notify",
    ]);
  });
});
