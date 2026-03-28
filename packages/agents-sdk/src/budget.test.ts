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
});
