import { describe, expect, it } from "vitest";
import type { DebugBundle } from "@/hooks/api/types";
import { buildRunTimeline, summarizeRunDebugBundle } from "./run-debug-utils";

const bundle: DebugBundle = {
  checkpoints: [
    {
      created_at: "2026-03-28T10:01:00Z",
      id: "cp-1",
      run_id: "run-1",
      sequence: 1,
      source: "sdk",
      state: { phase: "planning" },
    },
  ],
  events: [
    {
      created_at: "2026-03-28T10:00:00Z",
      id: "evt-1",
      level: "info",
      message: "started",
      run_id: "run-1",
      type: "run.started",
    },
  ],
  outputs: [],
  resource_snapshots: [],
  run: {
    attempt: 1,
    created_at: "2026-03-28T10:00:00Z",
    debug_mode: false,
    error: undefined,
    execution_trace: undefined,
    finished_at: "2026-03-28T10:03:00Z",
    id: "run-1",
    job_id: "job-1",
    job_version: 1,
    lineage_depth: 0,
    payload: null,
    priority: 0,
    project_id: "proj-1",
    result: null,
    started_at: "2026-03-28T10:00:01Z",
    status: "completed",
    triggered_by: "manual",
  },
  tool_calls: [
    {
      created_at: "2026-03-28T10:02:00Z",
      duration_ms: 120,
      id: "tool-1",
      input: { query: "agents" },
      output: { count: 2 },
      run_id: "run-1",
      status: "completed",
      tool_name: "search",
    },
  ],
  usage: [
    {
      completion_tokens: 10,
      cost_microusd: 250,
      created_at: "2026-03-28T10:02:30Z",
      id: "usage-1",
      model: "gpt-5.4",
      prompt_tokens: 20,
      provider: "openai",
      run_id: "run-1",
      total_tokens: 30,
    },
  ],
};

describe("run-debug-utils", () => {
  it("builds a single sorted timeline across events, checkpoints, usage, and tool calls", () => {
    expect(buildRunTimeline(bundle)).toEqual([
      {
        created_at: "2026-03-28T10:00:00Z",
        kind: "event",
        label: "run.started",
        level: "info",
        message: "started",
        type: "run.started",
      },
      {
        created_at: "2026-03-28T10:01:00Z",
        kind: "checkpoint",
        label: "Checkpoint #1",
        sequence: 1,
        state: { phase: "planning" },
      },
      {
        created_at: "2026-03-28T10:02:00Z",
        duration_ms: 120,
        kind: "tool_call",
        label: "search",
        status: "completed",
        tool_name: "search",
      },
      {
        cost_microusd: 250,
        created_at: "2026-03-28T10:02:30Z",
        kind: "usage",
        label: "openai:gpt-5.4",
        model: "gpt-5.4",
        provider: "openai",
        total_tokens: 30,
      },
    ]);
  });

  it("summarizes costs, tools, and checkpoints", () => {
    expect(summarizeRunDebugBundle(bundle)).toEqual({
      checkpoint_count: 1,
      latest_checkpoint: { phase: "planning" },
      model_breakdown: [
        {
          cost_microusd: 250,
          label: "gpt-5.4",
          total_tokens: 30,
        },
      ],
      tool_breakdown: [
        {
          count: 1,
          failed_count: 0,
          tool_name: "search",
        },
      ],
      total_cost_microusd: 250,
      total_tokens: 30,
      usage_count: 1,
    });
  });
});
