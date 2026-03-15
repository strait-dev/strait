import { createActor } from "xstate";
import { runMachine, type RunEvent } from "./run-machine";

export { runMachine, type RunEvent } from "./run-machine";
export {
  workflowRunMachine,
  type WorkflowRunEvent,
} from "./workflow-run-machine";
export { stepRunMachine, type StepRunEvent } from "./step-run-machine";

/** All valid run status values. */
export type RunStatus =
  | "delayed"
  | "queued"
  | "dequeued"
  | "executing"
  | "waiting"
  | "completed"
  | "failed"
  | "timed_out"
  | "crashed"
  | "system_failed"
  | "canceled"
  | "expired"
  | "dead_letter"
  | "replay_staged";

/** All valid workflow run status values. */
export type WorkflowRunStatus =
  | "pending"
  | "running"
  | "paused"
  | "completed"
  | "failed"
  | "timed_out"
  | "canceled";

/** All valid step run status values. */
export type StepRunStatus =
  | "pending"
  | "waiting"
  | "running"
  | "completed"
  | "failed"
  | "skipped"
  | "canceled";

const terminalRunStatuses = new Set<RunStatus>([
  "completed",
  "failed",
  "timed_out",
  "crashed",
  "system_failed",
  "canceled",
  "expired",
]);

/**
 * Check if a transition is valid for a job run.
 *
 * @param from - Current run status.
 * @param event - Event to attempt.
 * @returns `true` if the transition is valid.
 *
 * @example
 * ```ts
 * isValidRunTransition("executing", "COMPLETE"); // true
 * isValidRunTransition("completed", "EXECUTE"); // false
 * ```
 */
export const isValidRunTransition = (
  from: RunStatus,
  event: RunEvent
): boolean => {
  const actor = createActor(runMachine);
  actor.start();

  // Walk machine to the `from` state by checking its initial state
  const snapshot = actor.getSnapshot();
  if (snapshot.value !== from) {
    // We need to check the machine definition directly
    const stateConfig = runMachine.config.states?.[from];
    if (!stateConfig) return false;
    const transitions =
      stateConfig.on as Record<string, string> | undefined;
    if (!transitions) return false;
    return event in transitions;
  }

  return false;
};

/**
 * Check if a transition is valid by inspecting the machine definition directly.
 * More reliable than actor-based approach.
 */
const getValidTransitions = (
  machineStates: Record<string, unknown>,
  from: string
): Record<string, string> => {
  const stateConfig = machineStates[from] as
    | { on?: Record<string, string> }
    | undefined;
  return (stateConfig?.on as Record<string, string>) ?? {};
};

/**
 * Check if a transition is valid for a job run by inspecting the machine definition.
 *
 * @example
 * ```ts
 * isValidRunTransition("executing", "COMPLETE"); // true
 * isValidRunTransition("completed", "EXECUTE"); // false
 * ```
 */
export const canTransitionRun = (
  from: RunStatus,
  event: RunEvent
): boolean => {
  const transitions = getValidTransitions(
    runMachine.config.states as Record<string, unknown>,
    from
  );
  return event in transitions;
};

/**
 * Check if a status is terminal (no further transitions possible).
 *
 * @example
 * ```ts
 * isTerminalRunStatus("completed"); // true
 * isTerminalRunStatus("executing"); // false
 * ```
 */
export const isTerminalRunStatus = (status: RunStatus): boolean =>
  terminalRunStatuses.has(status);
