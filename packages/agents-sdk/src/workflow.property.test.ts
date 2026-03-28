import fc from "fast-check";
import { describe, expect, it } from "vitest";

import { createDynamicSteps, fanOutSteps } from "./workflow";

const nonEmptyStringArbitrary = fc
  .string({ minLength: 1 })
  .filter((value) => value.trim().length > 0);

describe("workflow helpers property coverage", () => {
  it("creates unique fan-out step refs for arbitrary worker counts", () => {
    fc.assert(
      fc.property(
        fc.uniqueArray(nonEmptyStringArbitrary, {
          minLength: 1,
          maxLength: 10,
        }),
        (agentIds) => {
          const steps = fanOutSteps({
            dependsOn: ["planner"],
            stepRefPrefix: "worker",
            synthesizer: {
              agentId: "synth",
              stepRef: "synthesis",
            },
            workers: agentIds.map((agentId) => ({ agentId })),
          });

          const stepRefs = steps.map((step) => step.step_ref);
          expect(new Set(stepRefs).size).toBe(stepRefs.length);
          expect(steps.at(-1)?.depends_on).toEqual(
            stepRefs.slice(0, Math.max(stepRefs.length - 1, 0))
          );
        }
      ),
      { numRuns: 50 }
    );
  });

  it("keeps dynamic step refs stable for already unique step sets", () => {
    fc.assert(
      fc.property(
        fc.uniqueArray(
          fc.record({
            agentId: nonEmptyStringArbitrary,
            stepRef: nonEmptyStringArbitrary,
          }),
          {
            maxLength: 10,
            minLength: 1,
            selector: (item) => item.stepRef.trim(),
          }
        ),
        (steps) => {
          const envelope = createDynamicSteps(
            steps.map((step) => ({
              agent_id: step.agentId,
              depends_on: ["planner"],
              step_ref: step.stepRef,
              step_type: "job" as const,
            })),
            { knownStepRefs: ["planner"], maxDynamicSteps: 20 }
          );

          expect(envelope.dynamic_steps.map((step) => step.step_ref)).toEqual(
            steps.map((step) => step.stepRef.trim())
          );
        }
      ),
      { numRuns: 50 }
    );
  });
});
