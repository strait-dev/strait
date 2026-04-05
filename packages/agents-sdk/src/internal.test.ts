import { describe, expect, it, vi } from "vitest";

import type { AdapterContext, AdapterTelemetryOptions } from "./internal";
import { reportCheckpointState } from "./internal";
import type { BudgetSnapshot, JsonValue } from "./types";

function mockContext(overrides: Partial<AdapterContext> = {}): AdapterContext {
  return {
    reportToolCall: vi.fn(async () => ({}) as never),
    reportUsage: vi.fn(async () => ({}) as never),
    stream: vi.fn(async () => ({}) as never),
    ...overrides,
  };
}

const testSnapshot: BudgetSnapshot = {
  promptTokens: 500,
  completionTokens: 200,
  totalTokens: 700,
  costMicrousd: 1500,
  toolCalls: 3,
  iterations: 2,
  limits: { maxTokens: 10_000, maxCostMicrousd: 50_000 },
};

describe("reportCheckpointState", () => {
  it("enriches checkpoint state with budget snapshot", async () => {
    const checkpoint = vi.fn(async () => ({}) as never);
    const ctx = mockContext({
      checkpoint,
      budgetSnapshot: () => testSnapshot,
    });

    const opts: AdapterTelemetryOptions["checkpoint"] = {
      onStepFinish: async () => ({ phase: "tool-result", iteration: 1 }),
      source: "test",
    };

    await reportCheckpointState(ctx, opts, {});

    expect(checkpoint).toHaveBeenCalledTimes(1);
    const callArgs = checkpoint.mock.calls[0] as unknown[];
    const [state, options] = callArgs;
    expect(state).toEqual(
      expect.objectContaining({
        phase: "tool-result",
        iteration: 1,
        cost_microusd_at_checkpoint: 1500,
        tokens_at_checkpoint: 700,
        iterations_at_checkpoint: 2,
      })
    );
    expect(options).toEqual({ source: "test" });
  });

  it("skips enrichment when budgetSnapshot is not available", async () => {
    const checkpoint = vi.fn(async () => ({}) as never);
    const ctx = mockContext({ checkpoint });

    const opts: AdapterTelemetryOptions["checkpoint"] = {
      onStepFinish: async () => ({ phase: "done" }),
      source: "test",
    };

    await reportCheckpointState(ctx, opts, {});

    const callArgs = checkpoint.mock.calls[0] as unknown[];
    // Should still have the original state, just without enrichment.
    expect(callArgs[0]).toEqual({ phase: "done" });
  });

  it("skips entirely when onStepFinish returns undefined", async () => {
    const checkpoint = vi.fn(async () => ({}) as never);
    const ctx = mockContext({
      checkpoint,
      budgetSnapshot: () => testSnapshot,
    });

    const opts: AdapterTelemetryOptions["checkpoint"] = {
      onStepFinish: async () => undefined as unknown as JsonValue,
      source: "test",
    };

    await reportCheckpointState(ctx, opts, {});

    expect(checkpoint).not.toHaveBeenCalled();
  });

  it("skips when checkpoint is not on context", async () => {
    const ctx = mockContext({
      budgetSnapshot: () => testSnapshot,
    });

    const opts: AdapterTelemetryOptions["checkpoint"] = {
      onStepFinish: async () => ({ step: 1 }),
      source: "test",
    };

    // Should not throw even without checkpoint function.
    await reportCheckpointState(ctx, opts, {});
  });

  it("does not enrich non-object state", async () => {
    const checkpoint = vi.fn(async () => ({}) as never);
    const ctx = mockContext({
      checkpoint,
      budgetSnapshot: () => testSnapshot,
    });

    const opts: AdapterTelemetryOptions["checkpoint"] = {
      onStepFinish: async () => "simple string" as unknown as JsonValue,
      source: "test",
    };

    await reportCheckpointState(ctx, opts, {});

    // String state should pass through without enrichment.
    const callArgs = checkpoint.mock.calls[0] as unknown[];
    expect(callArgs[0]).toBe("simple string");
  });
});
