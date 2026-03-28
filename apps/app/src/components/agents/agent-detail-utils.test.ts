import { describe, expect, it } from "vitest";
import type { JobRun, RunStatus } from "@/hooks/api/types";
import { summarizeAgentRuns } from "./agent-detail-utils";

function makeRun(
  id: string,
  status: RunStatus,
  createdAt: string
): JobRun {
  return {
    attempt: 1,
    created_at: createdAt,
    debug_mode: false,
    error: undefined,
    execution_trace: undefined,
    finished_at: undefined,
    id,
    job_id: "job_123",
    job_version: 1,
    lineage_depth: 0,
    payload: null,
    priority: 0,
    project_id: "project_123",
    result: null,
    started_at: undefined,
    status,
    triggered_by: "manual",
  };
}

describe("summarizeAgentRuns", () => {
  it("counts successful, failed, and active runs", () => {
    const summary = summarizeAgentRuns([
      makeRun("run_completed", "completed", "2026-03-28T08:00:00Z"),
      makeRun("run_failed", "failed", "2026-03-28T09:00:00Z"),
      makeRun("run_active", "executing", "2026-03-28T10:00:00Z"),
      makeRun("run_dead_letter", "dead_letter", "2026-03-28T11:00:00Z"),
    ]);

    expect(summary.totalRuns).toBe(4);
    expect(summary.successfulRuns).toBe(1);
    expect(summary.failedRuns).toBe(2);
    expect(summary.activeRuns).toBe(1);
  });

  it("returns the latest run by created_at", () => {
    const summary = summarizeAgentRuns([
      makeRun("run_old", "queued", "2026-03-28T07:00:00Z"),
      makeRun("run_new", "completed", "2026-03-28T12:00:00Z"),
    ]);

    expect(summary.latestRun?.id).toBe("run_new");
  });

  it("handles empty input", () => {
    expect(summarizeAgentRuns([])).toEqual({
      activeRuns: 0,
      failedRuns: 0,
      latestRun: null,
      successfulRuns: 0,
      totalRuns: 0,
    });
  });
});
