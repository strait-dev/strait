import { describe, expect, it } from "vitest";
import type { JobRun, RunStatus, RunUsage } from "@/hooks/api/types";
import { buildAgentCostSummary } from "./agent-cost-utils";

function makeRun(
  id: string,
  createdAt: string,
  status: RunStatus = "completed"
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

function makeUsage(
  id: string,
  runId: string,
  provider: string,
  costMicrousd: number,
  totalTokens: number,
  createdAt: string
): RunUsage {
  return {
    completion_tokens: Math.floor(totalTokens / 3),
    cost_microusd: costMicrousd,
    created_at: createdAt,
    id,
    model: "gpt-5",
    prompt_tokens: Math.floor((totalTokens * 2) / 3),
    provider,
    run_id: runId,
    total_tokens: totalTokens,
  };
}

describe("buildAgentCostSummary", () => {
  it("aggregates totals, providers, and latest run cost", () => {
    const runs = [
      makeRun("run_1", "2026-03-27T10:00:00Z"),
      makeRun("run_2", "2026-03-28T10:00:00Z"),
    ];
    const usage = [
      makeUsage(
        "usage_1",
        "run_1",
        "openai",
        1200,
        900,
        "2026-03-27T10:01:00Z"
      ),
      makeUsage("usage_2", "run_2", "openai", 800, 600, "2026-03-28T10:01:00Z"),
      makeUsage(
        "usage_3",
        "run_2",
        "anthropic",
        500,
        400,
        "2026-03-28T10:02:00Z"
      ),
    ];

    const summary = buildAgentCostSummary(runs, usage);

    expect(summary.total_cost_microusd).toBe(2500);
    expect(summary.total_tokens).toBe(1900);
    expect(summary.average_cost_microusd).toBe(1250);
    expect(summary.latest_run_cost_microusd).toBe(1300);
    expect(summary.providers).toEqual([
      { cost_microusd: 2000, provider: "openai", total_tokens: 1500 },
      { cost_microusd: 500, provider: "anthropic", total_tokens: 400 },
    ]);
  });

  it("groups daily totals in ascending date order", () => {
    const summary = buildAgentCostSummary(
      [makeRun("run_1", "2026-03-28T10:00:00Z")],
      [
        makeUsage(
          "usage_2",
          "run_1",
          "openai",
          500,
          400,
          "2026-03-29T10:01:00Z"
        ),
        makeUsage(
          "usage_1",
          "run_1",
          "openai",
          250,
          200,
          "2026-03-28T10:01:00Z"
        ),
      ]
    );

    expect(summary.daily).toEqual([
      { cost_microusd: 250, date: "2026-03-28", total_tokens: 200 },
      { cost_microusd: 500, date: "2026-03-29", total_tokens: 400 },
    ]);
  });

  it("returns zeroed metrics for empty usage", () => {
    expect(buildAgentCostSummary([], [])).toEqual({
      average_cost_microusd: 0,
      daily: [],
      latest_run_cost_microusd: 0,
      providers: [],
      total_cost_microusd: 0,
      total_tokens: 0,
      usage_records: 0,
    });
  });
});
