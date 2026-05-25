/**
 * Pure helpers for the workflow continue-as-new feature.
 *
 * Kept free of React and network concerns so the precondition and
 * input-parsing logic shared by the action dialog can be unit tested in
 * isolation. Mirrors the server contract in
 * `apps/strait/internal/domain` (ContinueVersionStrategy) and the
 * `POST /v1/workflow-runs/{id}/continue-as-new` precondition.
 */

import type { WorkflowRun, WorkflowRunStatus } from "@/hooks/api/types";

/** Version-resolution strategies the successor run can use. */
export const CONTINUE_VERSION_STRATEGIES = ["repin", "latest"] as const;
export type ContinueVersionStrategy =
  (typeof CONTINUE_VERSION_STRATEGIES)[number];

/** Server default: repin keeps the chain deterministic across deploys. */
export const DEFAULT_CONTINUE_VERSION_STRATEGY: ContinueVersionStrategy =
  "repin";

/** Statuses a run can be continued from (mirrors the server precondition). */
const CONTINUABLE_STATUSES: ReadonlySet<WorkflowRunStatus> = new Set([
  "running",
  "paused",
]);

/** True when continue-as-new is a valid action for the given run status. */
export function canContinueWorkflowRun(
  status: WorkflowRunStatus | string
): boolean {
  return CONTINUABLE_STATUSES.has(status as WorkflowRunStatus);
}

/**
 * True when the run participates in a continue-as-new lineage, either as a
 * successor (continued_from set or depth above the root) or a predecessor
 * (continued_to set). Used to decide whether to offer chain navigation.
 */
export function isPartOfChain(
  run: Pick<
    WorkflowRun,
    | "continued_from_workflow_run_id"
    | "continued_to_workflow_run_id"
    | "lineage_depth"
  >
): boolean {
  return (
    Boolean(run.continued_from_workflow_run_id) ||
    Boolean(run.continued_to_workflow_run_id) ||
    (run.lineage_depth ?? 0) > 0
  );
}

export type ParsedContinueInput =
  | { ok: true; value: unknown }
  | { ok: false; error: string };

/**
 * Parses the optional carry-over input a user types into the dialog. An
 * empty or whitespace-only string means "no input" (value undefined);
 * otherwise the text must be valid JSON, matching how the server forwards
 * the payload opaquely to the successor run.
 */
export function parseContinueInput(raw: string): ParsedContinueInput {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return { ok: true, value: undefined };
  }
  try {
    return { ok: true, value: JSON.parse(trimmed) as unknown };
  } catch {
    return { ok: false, error: "Input must be valid JSON." };
  }
}
