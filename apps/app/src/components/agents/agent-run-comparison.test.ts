import { describe, expect, it } from "vitest";
import type { AgentRunComparison } from "./agent-run-comparison";

function makeComparison(
  overrides: Partial<AgentRunComparison> = {}
): AgentRunComparison {
  return {
    run_a: {
      run_id: "run-a",
      status: "completed",
      model: "gpt-5.4",
      total_tokens: 1000,
      cost_microusd: 500,
      duration_secs: 10,
      tool_call_count: 3,
      attempt: 1,
    },
    run_b: {
      run_id: "run-b",
      status: "completed",
      model: "gpt-5.4",
      total_tokens: 1000,
      cost_microusd: 500,
      duration_secs: 10,
      tool_call_count: 3,
      attempt: 1,
    },
    cost_diff_microusd: 0,
    token_diff: 0,
    duration_diff_secs: 0,
    status_match: true,
    model_match: true,
    ...overrides,
  };
}

describe("agent-run-comparison types", () => {
  it("represents identical runs with zero diffs", () => {
    const comp = makeComparison();
    expect(comp.cost_diff_microusd).toBe(0);
    expect(comp.token_diff).toBe(0);
    expect(comp.status_match).toBe(true);
    expect(comp.model_match).toBe(true);
  });

  it("represents cost diff correctly", () => {
    const comp = makeComparison({
      run_a: {
        ...makeComparison().run_a,
        cost_microusd: 1000,
      },
      run_b: {
        ...makeComparison().run_b,
        cost_microusd: 300,
      },
      cost_diff_microusd: 700,
    });
    expect(comp.cost_diff_microusd).toBe(700);
  });

  it("represents different models", () => {
    const comp = makeComparison({
      run_a: { ...makeComparison().run_a, model: "claude-sonnet-4-6" },
      run_b: { ...makeComparison().run_b, model: "gpt-5.4-mini" },
      model_match: false,
    });
    expect(comp.model_match).toBe(false);
  });

  it("represents tool call diffs", () => {
    const comp = makeComparison({
      tool_call_diffs: [
        { tool_name: "search", count_a: 5, count_b: 2 },
        { tool_name: "analyze", count_a: 1, count_b: 3 },
      ],
    });
    expect(comp.tool_call_diffs).toHaveLength(2);
    const searchDiff = comp.tool_call_diffs?.find(
      (d) => d.tool_name === "search"
    );
    expect(searchDiff?.count_a).toBe(5);
    expect(searchDiff?.count_b).toBe(2);
  });

  it("represents different statuses", () => {
    const comp = makeComparison({
      run_a: { ...makeComparison().run_a, status: "completed" },
      run_b: {
        ...makeComparison().run_b,
        status: "failed",
        error_class: "oom",
      },
      status_match: false,
    });
    expect(comp.status_match).toBe(false);
    expect(comp.run_b.error_class).toBe("oom");
  });

  it("handles empty tool call diffs", () => {
    const comp = makeComparison({ tool_call_diffs: [] });
    expect(comp.tool_call_diffs).toHaveLength(0);
  });
});
