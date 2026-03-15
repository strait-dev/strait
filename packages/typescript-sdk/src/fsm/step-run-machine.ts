import { setup } from "xstate";

/**
 * XState machine modeling all valid StepRun state transitions.
 *
 * States: pending → waiting/running → completed/failed/...
 *
 * @example
 * ```ts
 * import { createActor } from "xstate";
 * import { stepRunMachine } from "@strait/ts";
 *
 * const actor = createActor(stepRunMachine);
 * actor.start();
 * actor.send({ type: "START" });
 * actor.getSnapshot().value; // "running"
 * ```
 */
export const stepRunMachine = setup({
  types: {
    events: {} as
      | { type: "WAIT" }
      | { type: "START" }
      | { type: "COMPLETE" }
      | { type: "FAIL" }
      | { type: "SKIP" }
      | { type: "CANCEL" },
  },
}).createMachine({
  id: "stepRun",
  initial: "pending",
  states: {
    pending: {
      on: {
        WAIT: "waiting",
        START: "running",
        SKIP: "skipped",
        CANCEL: "canceled",
        COMPLETE: "completed",
      },
    },
    waiting: {
      on: {
        START: "running",
        SKIP: "skipped",
        CANCEL: "canceled",
        COMPLETE: "completed",
      },
    },
    running: {
      on: {
        COMPLETE: "completed",
        FAIL: "failed",
        CANCEL: "canceled",
      },
    },
    completed: { type: "final" },
    failed: { type: "final" },
    skipped: { type: "final" },
    canceled: { type: "final" },
  },
});

/** All possible events for the step run FSM. */
export type StepRunEvent =
  | "WAIT"
  | "START"
  | "COMPLETE"
  | "FAIL"
  | "SKIP"
  | "CANCEL";
