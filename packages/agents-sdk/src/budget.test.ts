import { describe, expect, it } from "vitest";

import { BudgetLedger } from "./budget";
import { BudgetExceededError } from "./errors";

const toolCallBudgetExceededMessage = "tool_calls budget exceeded";

describe("BudgetLedger", () => {
  it("tracks cumulative usage totals", () => {
    const ledger = new BudgetLedger({
      maxCostMicrousd: 1000,
      maxTokens: 1000,
      maxToolCalls: 10,
    });

    ledger.recordUsage({
      provider: "local",
      model: "local-agent",
      promptTokens: 10,
      completionTokens: 5,
      totalTokens: 15,
      costMicrousd: 150,
    });
    ledger.recordToolCall();

    expect(ledger.snapshot()).toEqual({
      promptTokens: 10,
      completionTokens: 5,
      totalTokens: 15,
      costMicrousd: 150,
      toolCalls: 1,
      iterations: 0,
      limits: {
        maxCostMicrousd: 1000,
        maxTokens: 1000,
        maxToolCalls: 10,
      },
    });
  });

  it("enforces token and cost budgets before updating state", () => {
    const ledger = new BudgetLedger({
      maxCostMicrousd: 100,
      maxTokens: 10,
    });

    ledger.recordUsage({
      provider: "local",
      model: "local-agent",
      promptTokens: 3,
      completionTokens: 3,
      totalTokens: 6,
      costMicrousd: 60,
    });

    expect(() =>
      ledger.recordUsage({
        provider: "local",
        model: "local-agent",
        promptTokens: 3,
        completionTokens: 3,
        totalTokens: 6,
        costMicrousd: 60,
      })
    ).toThrow(BudgetExceededError);

    expect(ledger.snapshot().totalTokens).toBe(6);
    expect(ledger.snapshot().costMicrousd).toBe(60);
  });

  it("enforces the tool call budget", () => {
    const ledger = new BudgetLedger({
      maxToolCalls: 1,
    });

    ledger.recordToolCall();

    expect(() => ledger.recordToolCall()).toThrow(
      toolCallBudgetExceededMessage
    );
    expect(ledger.snapshot().toolCalls).toBe(1);
  });

  it("fails fast when a new call starts after the cost budget is exhausted", () => {
    const ledger = new BudgetLedger({
      maxCostMicrousd: 100,
    });

    ledger.recordUsage({
      provider: "local",
      model: "local-agent",
      promptTokens: 4,
      completionTokens: 6,
      totalTokens: 10,
      costMicrousd: 100,
    });

    expect(() => ledger.assertWithinLimits()).toThrow(BudgetExceededError);
  });

  it("enforces the iteration budget", () => {
    const ledger = new BudgetLedger({
      maxIterations: 3,
    });

    ledger.recordIteration();
    ledger.recordIteration();
    ledger.recordIteration();

    expect(() => ledger.recordIteration()).toThrow(BudgetExceededError);
    expect(() => ledger.recordIteration()).toThrow(
      "iterations budget exceeded"
    );
    expect(ledger.snapshot().iterations).toBe(3);
  });

  it("tracks iterations in snapshot", () => {
    const ledger = new BudgetLedger({ maxIterations: 10 });

    ledger.recordIteration();
    ledger.recordIteration();

    const snap = ledger.snapshot();
    expect(snap.iterations).toBe(2);
    expect(snap.limits.maxIterations).toBe(10);
  });

  it("assertWithinLimits catches iteration limit", () => {
    const ledger = new BudgetLedger({ maxIterations: 1 });

    ledger.recordIteration();
    expect(() => ledger.assertWithinLimits()).toThrow(BudgetExceededError);
  });

  it("allows unlimited iterations when maxIterations is not set", () => {
    const ledger = new BudgetLedger({});

    for (let i = 0; i < 100; i++) {
      ledger.recordIteration();
    }
    expect(ledger.snapshot().iterations).toBe(100);
  });

  it("maxIterations = 1 allows exactly one iteration", () => {
    const ledger = new BudgetLedger({ maxIterations: 1 });

    ledger.recordIteration();
    expect(ledger.snapshot().iterations).toBe(1);
    expect(() => ledger.recordIteration()).toThrow(BudgetExceededError);
  });
});
