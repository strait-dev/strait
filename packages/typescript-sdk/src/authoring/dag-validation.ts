import { DagValidationError } from "../errors";
import type { WorkflowStepDefinition } from "./steps";

/**
 * Validates a DAG of workflow steps using Kahn's algorithm for topological sorting.
 *
 * Detects:
 * - Circular dependencies between steps
 * - References to non-existent step refs in `dependsOn`
 * - Duplicate step refs
 *
 * @param steps - Array of workflow step definitions to validate.
 * @returns A topologically sorted array of step refs.
 * @throws {DagValidationError} If the DAG is invalid.
 *
 * @example
 * ```ts
 * const steps = [
 *   step.job("a", "job_1"),
 *   step.job("b", "job_2", { dependsOn: ["a"] }),
 *   step.job("c", "job_3", { dependsOn: ["a", "b"] }),
 * ];
 * const sorted = validateDag(steps);
 * // sorted = ["a", "b", "c"] — topological order
 * ```
 */
const checkDuplicateRefs = (refs: readonly string[]): void => {
  const seen = new Set<string>();
  const duplicateRefs: string[] = [];

  for (const ref of refs) {
    if (seen.has(ref)) {
      duplicateRefs.push(ref);
    }
    seen.add(ref);
  }

  if (duplicateRefs.length > 0) {
    throw new DagValidationError({
      message: `Duplicate step refs: ${duplicateRefs.join(", ")}`,
      duplicateRefs,
    });
  }
};

const checkMissingRefs = (
  steps: readonly WorkflowStepDefinition[],
  allRefs: ReadonlySet<string>
): void => {
  const missingRefs: string[] = [];

  for (const s of steps) {
    for (const dep of s.dependsOn ?? []) {
      if (!allRefs.has(dep)) {
        missingRefs.push(dep);
      }
    }
  }

  if (missingRefs.length > 0) {
    throw new DagValidationError({
      message: `References to non-existent steps: ${missingRefs.join(", ")}`,
      missingRefs,
    });
  }
};

const topologicalSort = (
  steps: readonly WorkflowStepDefinition[],
  refs: readonly string[]
): readonly string[] => {
  const inDegree = new Map<string, number>();
  const adjacency = new Map<string, string[]>();

  for (const ref of refs) {
    inDegree.set(ref, 0);
    adjacency.set(ref, []);
  }

  for (const s of steps) {
    for (const dep of s.dependsOn ?? []) {
      adjacency.get(dep)?.push(s.stepRef);
      inDegree.set(s.stepRef, (inDegree.get(s.stepRef) ?? 0) + 1);
    }
  }

  const queue: string[] = [];
  for (const [ref, degree] of inDegree) {
    if (degree === 0) {
      queue.push(ref);
    }
  }

  const sorted: string[] = [];

  while (queue.length > 0) {
    const node = queue.shift() as string;
    sorted.push(node);

    for (const neighbor of adjacency.get(node) ?? []) {
      const newDegree = (inDegree.get(neighbor) ?? 1) - 1;
      inDegree.set(neighbor, newDegree);
      if (newDegree === 0) {
        queue.push(neighbor);
      }
    }
  }

  if (sorted.length !== steps.length) {
    const inCycle = refs.filter((ref) => !sorted.includes(ref));
    throw new DagValidationError({
      message: `Circular dependency detected involving steps: ${inCycle.join(", ")}`,
      cycles: inCycle,
    });
  }

  return sorted;
};

export const validateDag = (
  steps: readonly WorkflowStepDefinition[]
): readonly string[] => {
  if (steps.length === 0) {
    return [];
  }

  const refs = steps.map((s) => s.stepRef);

  checkDuplicateRefs(refs);
  checkMissingRefs(steps, new Set(refs));

  return topologicalSort(steps, refs);
};
