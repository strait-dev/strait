import { setup } from "xstate";

/**
 * XState machine modeling all valid JobRun state transitions.
 *
 * Mirrors the server-side FSM. Use this to:
 * - Validate transitions client-side before API calls
 * - Build UI state indicators
 * - Understand the run lifecycle
 *
 * States: delayed → queued → dequeued → executing → completed/failed/...
 *
 * @example
 * ```ts
 * import { createActor } from "xstate";
 * import { runMachine } from "@strait/ts";
 *
 * const actor = createActor(runMachine, { input: { initialStatus: "queued" } });
 * actor.start();
 * actor.send({ type: "DEQUEUE" });
 * actor.getSnapshot().value; // "dequeued"
 * ```
 */
export const runMachine = setup({
  types: {
    events: {} as
      | { type: "ENQUEUE" }
      | { type: "DEQUEUE" }
      | { type: "EXECUTE" }
      | { type: "COMPLETE" }
      | { type: "FAIL" }
      | { type: "TIMEOUT" }
      | { type: "CRASH" }
      | { type: "SYSTEM_FAIL" }
      | { type: "CANCEL" }
      | { type: "EXPIRE" }
      | { type: "WAIT" }
      | { type: "REQUEUE" }
      | { type: "DEAD_LETTER" }
      | { type: "REPLAY" },
  },
}).createMachine({
  id: "jobRun",
  initial: "delayed",
  states: {
    delayed: {
      on: {
        ENQUEUE: "queued",
        CANCEL: "canceled",
        EXPIRE: "expired",
      },
    },
    queued: {
      on: {
        DEQUEUE: "dequeued",
        CANCEL: "canceled",
        EXPIRE: "expired",
      },
    },
    dequeued: {
      on: {
        EXECUTE: "executing",
        REQUEUE: "queued",
        CANCEL: "canceled",
        SYSTEM_FAIL: "system_failed",
      },
    },
    executing: {
      on: {
        COMPLETE: "completed",
        FAIL: "failed",
        TIMEOUT: "timed_out",
        CRASH: "crashed",
        CANCEL: "canceled",
        WAIT: "waiting",
        REQUEUE: "queued",
        SYSTEM_FAIL: "system_failed",
        DEAD_LETTER: "dead_letter",
      },
    },
    waiting: {
      on: {
        EXECUTE: "executing",
        COMPLETE: "completed",
        FAIL: "failed",
        CANCEL: "canceled",
        TIMEOUT: "timed_out",
      },
    },
    dead_letter: {
      on: {
        REQUEUE: "queued",
        REPLAY: "replay_staged",
      },
    },
    replay_staged: {
      on: {
        ENQUEUE: "queued",
        CANCEL: "canceled",
      },
    },
    completed: { type: "final" },
    failed: { type: "final" },
    timed_out: { type: "final" },
    crashed: { type: "final" },
    system_failed: { type: "final" },
    canceled: { type: "final" },
    expired: { type: "final" },
  },
});

/** All possible events for the job run FSM. */
export type RunEvent =
  | "ENQUEUE"
  | "DEQUEUE"
  | "EXECUTE"
  | "COMPLETE"
  | "FAIL"
  | "TIMEOUT"
  | "CRASH"
  | "SYSTEM_FAIL"
  | "CANCEL"
  | "EXPIRE"
  | "WAIT"
  | "REQUEUE"
  | "DEAD_LETTER"
  | "REPLAY";
