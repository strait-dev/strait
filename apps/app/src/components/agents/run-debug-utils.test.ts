import { describe, expect, it } from "vitest";
import type { DebugBundle } from "@/hooks/api/types";
import {
  buildRunTimeline,
  extractErrorClassification,
  summarizeRunDebugBundle,
} from "./run-debug-utils";

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
      input: { query: "agents", sandbox_mode: "dynamic_worker" },
      output: {
        count: 2,
        outbound_reason: "host_not_allowlisted",
        sandbox_executor: "dynamic_worker",
      },
      run_id: "run-1",
      status: "blocked",
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
        outbound_reason: "host_not_allowlisted",
        sandbox_executor: "dynamic_worker",
        sandbox_mode: "dynamic_worker",
        status: "blocked",
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
      blocked_reason_breakdown: [
        {
          count: 1,
          reason: "host_not_allowlisted",
        },
      ],
      blocked_tool_call_count: 1,
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
          blocked_count: 1,
          count: 1,
          executors: ["dynamic_worker"],
          failed_count: 1,
          tool_name: "search",
        },
      ],
      tool_details: [
        {
          created_at: "2026-03-28T10:02:00Z",
          duration_ms: 120,
          outbound_reason: "host_not_allowlisted",
          sandbox_executor: "dynamic_worker",
          sandbox_mode: "dynamic_worker",
          status: "blocked",
          tool_name: "search",
        },
      ],
      tool_executor_breakdown: [
        {
          blocked_count: 1,
          count: 1,
          executor: "dynamic_worker",
        },
      ],
      total_cost_microusd: 250,
      total_tokens: 30,
      usage_count: 1,
    });
  });
});

describe("extractErrorClassification", () => {
  it("returns null when no error events exist", () => {
    expect(extractErrorClassification(bundle)).toBeNull();
  });

  it("extracts oom classification from error event data", () => {
    const oomBundle: DebugBundle = {
      ...bundle,
      events: [
        {
          created_at: "2026-03-28T10:00:00Z",
          data: {
            error: "Worker exceeded resource limits",
            error_class: "oom",
            suggestion: "Reduce tool complexity.",
          },
          id: "evt-err",
          level: "error",
          message: "agent runtime failed",
          run_id: "run-1",
          type: "error",
        },
      ],
    };
    const result = extractErrorClassification(oomBundle);
    expect(result).toEqual({
      error_class: "oom",
      suggestion: "Reduce tool complexity.",
    });
  });

  it("extracts timeout classification", () => {
    const timeoutBundle: DebugBundle = {
      ...bundle,
      events: [
        {
          created_at: "2026-03-28T10:00:00Z",
          data: { error_class: "timeout", suggestion: "Increase timeout." },
          id: "evt-timeout",
          level: "error",
          message: "agent runtime failed",
          run_id: "run-1",
          type: "error",
        },
      ],
    };
    const result = extractErrorClassification(timeoutBundle);
    expect(result).toEqual({
      error_class: "timeout",
      suggestion: "Increase timeout.",
    });
  });

  it("returns null when event data has no error_class", () => {
    const noClassBundle: DebugBundle = {
      ...bundle,
      events: [
        {
          created_at: "2026-03-28T10:00:00Z",
          data: { error: "something" },
          id: "evt-plain",
          level: "error",
          message: "agent runtime failed",
          run_id: "run-1",
          type: "error",
        },
      ],
    };
    expect(extractErrorClassification(noClassBundle)).toBeNull();
  });

  it("returns classification without suggestion when suggestion is missing", () => {
    const noSuggestionBundle: DebugBundle = {
      ...bundle,
      events: [
        {
          created_at: "2026-03-28T10:00:00Z",
          data: { error_class: "runtime_error" },
          id: "evt-runtime",
          level: "error",
          message: "agent runtime failed",
          run_id: "run-1",
          type: "error",
        },
      ],
    };
    const result = extractErrorClassification(noSuggestionBundle);
    expect(result).toEqual({
      error_class: "runtime_error",
      suggestion: undefined,
    });
  });
});
