import { describe, expect, it } from "vitest";
import type {
  JobRun,
  RunStatus,
  RunToolCall,
  RunUsage,
} from "@/hooks/api/types";
import {
  buildAgentCostSummary,
  readAgentBudgetLimitMicrousd,
} from "./agent-cost-utils";

function makeRun(
  id: string,
  createdAt: string,
  status: RunStatus = "completed",
  startedAt = createdAt,
  finishedAt = createdAt
): JobRun {
  return {
    attempt: 1,
    created_at: createdAt,
    debug_mode: false,
    error: undefined,
    execution_trace: undefined,
    finished_at: finishedAt,
    id,
    job_id: "job_123",
    job_version: 1,
    lineage_depth: 0,
    payload: null,
    priority: 0,
    project_id: "project_123",
    result: null,
    started_at: startedAt,
    status,
    triggered_by: "manual",
  };
}

function makeUsage(
  id: string,
  runId: string,
  provider: string,
  model: string,
  costMicrousd: number,
  totalTokens: number,
  createdAt: string
): RunUsage {
  return {
    completion_tokens: Math.floor(totalTokens / 3),
    cost_microusd: costMicrousd,
    created_at: createdAt,
    id,
    model,
    prompt_tokens: Math.floor((totalTokens * 2) / 3),
    provider,
    run_id: runId,
    total_tokens: totalTokens,
  };
}

function makeToolCall(
  id: string,
  runId: string,
  toolName: string,
  status: string,
  durationMs: number
): RunToolCall {
  return {
    created_at: "2026-03-28T10:01:00Z",
    duration_ms: durationMs,
    id,
    input: { ok: true },
    output: { ok: true },
    run_id: runId,
    status,
    tool_name: toolName,
  };
}

describe("buildAgentCostSummary", () => {
  it("aggregates totals, providers, models, tools, and latest run cost", () => {
    const runs = [
      makeRun(
        "run_1",
        "2026-03-27T10:00:00Z",
        "completed",
        "2026-03-27T10:00:00Z",
        "2026-03-27T10:03:00Z"
      ),
      makeRun(
        "run_2",
        "2026-03-28T10:00:00Z",
        "failed",
        "2026-03-28T10:00:00Z",
        "2026-03-28T10:01:30Z"
      ),
    ];
    const usage = [
      makeUsage(
        "usage_1",
        "run_1",
        "openai",
        "gpt-5",
        1200,
        900,
        "2026-03-27T10:01:00Z"
      ),
      makeUsage(
        "usage_2",
        "run_2",
        "openai",
        "gpt-5-mini",
        800,
        600,
        "2026-03-28T10:01:00Z"
      ),
      makeUsage(
        "usage_3",
        "run_2",
        "anthropic",
        "claude-sonnet-4-5",
        500,
        400,
        "2026-03-28T10:02:00Z"
      ),
    ];
    const toolCalls = [
      makeToolCall("tool_1", "run_1", "search", "completed", 120),
      makeToolCall("tool_2", "run_2", "search", "failed", 180),
      makeToolCall("tool_3", "run_2", "summarize", "completed", 80),
    ];

    const summary = buildAgentCostSummary(runs, usage, toolCalls, {
      budget: "$5.00",
    });

    expect(summary.total_cost_microusd).toBe(2500);
    expect(summary.total_tokens).toBe(1900);
    expect(summary.average_cost_microusd).toBe(1250);
    expect(summary.latest_run_cost_microusd).toBe(1300);
    expect(summary.average_duration_ms).toBe(135_000);
    expect(summary.average_tokens_per_run).toBe(950);
    expect(summary.providers).toEqual([
      { cost_microusd: 2000, label: "openai", total_tokens: 1500 },
      { cost_microusd: 500, label: "anthropic", total_tokens: 400 },
    ]);
    expect(summary.models).toEqual([
      { cost_microusd: 1200, label: "gpt-5", total_tokens: 900 },
      { cost_microusd: 800, label: "gpt-5-mini", total_tokens: 600 },
      {
        cost_microusd: 500,
        label: "claude-sonnet-4-5",
        total_tokens: 400,
      },
    ]);
    expect(summary.tools).toEqual([
      {
        average_duration_ms: 150,
        count: 2,
        failed_count: 1,
        tool_name: "search",
      },
      {
        average_duration_ms: 80,
        count: 1,
        failed_count: 0,
        tool_name: "summarize",
      },
    ]);
    expect(summary.budget_limit_microusd).toBe(5_000_000);
    expect(summary.budget_utilization_ratio).toBeCloseTo(0.0005, 6);
    expect(summary.run_breakdown[0]).toMatchObject({
      cost_microusd: 1300,
      run_id: "run_2",
      tool_calls: 2,
      total_tokens: 1000,
    });
  });

  it("groups daily totals in ascending date order and forecasts spend", () => {
    const summary = buildAgentCostSummary(
      [makeRun("run_1", "2026-03-28T10:00:00Z")],
      [
        makeUsage(
          "usage_2",
          "run_1",
          "openai",
          "gpt-5",
          500,
          400,
          "2026-03-29T10:01:00Z"
        ),
        makeUsage(
          "usage_1",
          "run_1",
          "openai",
          "gpt-5",
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
    expect(summary.forecast).toEqual({
      projected_daily_cost_microusd: 375,
      projected_monthly_cost_microusd: 11_250,
    });
  });

  it("returns zeroed metrics for empty usage", () => {
    expect(buildAgentCostSummary([], [])).toEqual({
      average_cost_microusd: 0,
      average_duration_ms: 0,
      average_tokens_per_run: 0,
      budget_limit_microusd: undefined,
      budget_utilization_ratio: undefined,
      daily: [],
      forecast: {
        projected_daily_cost_microusd: 0,
        projected_monthly_cost_microusd: 0,
      },
      latest_run_cost_microusd: 0,
      models: [],
      providers: [],
      run_breakdown: [],
      tools: [],
      total_cost_microusd: 0,
      total_tokens: 0,
      usage_records: 0,
    });
  });
});

describe("readAgentBudgetLimitMicrousd", () => {
  it("reads string, numeric, and object budget shapes", () => {
    expect(readAgentBudgetLimitMicrousd({ budget: "$5.50" })).toBe(5_500_000);
    expect(readAgentBudgetLimitMicrousd({ budget: 500_000 })).toBe(500_000);
    expect(
      readAgentBudgetLimitMicrousd({
        budget: { maxCostMicrousd: 1_250_000 },
      })
    ).toBe(1_250_000);
  });

  it("returns undefined for unsupported shapes", () => {
    expect(readAgentBudgetLimitMicrousd(null)).toBeUndefined();
    expect(
      readAgentBudgetLimitMicrousd({ budget: "not-a-budget" })
    ).toBeUndefined();
  });
});
