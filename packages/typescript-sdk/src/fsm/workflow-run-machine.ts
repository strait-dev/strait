import { setup } from "xstate";

/**
 * XState machine modeling all valid WorkflowRun state transitions.
 *
 * States: pending → running → completed/failed/...
 *
 * @example
 * ```ts
 * import { createActor } from "xstate";
 * import { workflowRunMachine } from "@strait/ts";
 *
 * const actor = createActor(workflowRunMachine);
 * actor.start();
 * actor.send({ type: "START" });
 * actor.getSnapshot().value; // "running"
 * ```
 */
export const workflowRunMachine = setup({
  types: {
    events: {} as
      | { type: "START" }
      | { type: "PAUSE" }
      | { type: "RESUME" }
      | { type: "COMPLETE" }
      | { type: "FAIL" }
      | { type: "TIMEOUT" }
      | { type: "CANCEL" },
  },
}).createMachine({
  id: "workflowRun",
  initial: "pending",
  states: {
    pending: {
      on: {
        START: "running",
        CANCEL: "canceled",
      },
    },
    running: {
      on: {
        PAUSE: "paused",
        COMPLETE: "completed",
        FAIL: "failed",
        TIMEOUT: "timed_out",
        CANCEL: "canceled",
      },
    },
    paused: {
      on: {
        RESUME: "running",
        COMPLETE: "completed",
        FAIL: "failed",
        TIMEOUT: "timed_out",
        CANCEL: "canceled",
      },
    },
    completed: { type: "final" },
    failed: { type: "final" },
    timed_out: { type: "final" },
    canceled: { type: "final" },
  },
});

/** All possible events for the workflow run FSM. */
export type WorkflowRunEvent =
  | "START"
  | "PAUSE"
  | "RESUME"
  | "COMPLETE"
  | "FAIL"
  | "TIMEOUT"
  | "CANCEL";
