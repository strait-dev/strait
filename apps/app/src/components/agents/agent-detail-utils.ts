import type { JobRun, RunStatus } from "@/hooks/api/types";

const SUCCESSFUL_STATUSES: ReadonlySet<RunStatus> = new Set(["completed"]);
const FAILED_STATUSES: ReadonlySet<RunStatus> = new Set([
  "failed",
  "timed_out",
  "crashed",
  "system_failed",
  "canceled",
  "expired",
  "dead_letter",
]);
const ACTIVE_STATUSES: ReadonlySet<RunStatus> = new Set([
  "delayed",
  "queued",
  "dequeued",
  "executing",
  "waiting",
]);

export type AgentRunSummary = {
  totalRuns: number;
  successfulRuns: number;
  failedRuns: number;
  activeRuns: number;
  latestRun: JobRun | null;
};

export function summarizeAgentRuns(runs: JobRun[]): AgentRunSummary {
  let successfulRuns = 0;
  let failedRuns = 0;
  let activeRuns = 0;
  let latestRun: JobRun | null = null;

  for (const run of runs) {
    const status = run.status as RunStatus;

    if (SUCCESSFUL_STATUSES.has(status)) {
      successfulRuns += 1;
    }

    if (FAILED_STATUSES.has(status)) {
      failedRuns += 1;
    }

    if (ACTIVE_STATUSES.has(status)) {
      activeRuns += 1;
    }

    if (!latestRun || run.created_at > latestRun.created_at) {
      latestRun = run;
    }
  }

  return {
    totalRuns: runs.length,
    successfulRuns,
    failedRuns,
    activeRuns,
    latestRun,
  };
}
