import { BudgetExceededError } from "./errors";
import type {
  AgentBudget,
  BudgetSnapshot,
  NormalizedUsageReport,
} from "./types";

type UsageTotals = Omit<BudgetSnapshot, "limits" | "toolCalls" | "iterations">;

function projectTotals(
  current: UsageTotals,
  usage: NormalizedUsageReport
): UsageTotals {
  return {
    promptTokens: current.promptTokens + usage.promptTokens,
    completionTokens: current.completionTokens + usage.completionTokens,
    totalTokens: current.totalTokens + usage.totalTokens,
    costMicrousd: current.costMicrousd + usage.costMicrousd,
  };
}

export class BudgetLedger {
  readonly #limits: AgentBudget;
  #totals: UsageTotals = {
    promptTokens: 0,
    completionTokens: 0,
    totalTokens: 0,
    costMicrousd: 0,
  };
  #toolCalls = 0;
  #iterations = 0;

  constructor(limits: AgentBudget = {}) {
    this.#limits = { ...limits };
  }

  recordUsage(usage: NormalizedUsageReport): BudgetSnapshot {
    const projected = projectTotals(this.#totals, usage);

    if (
      this.#limits.maxTokens != null &&
      this.#limits.maxTokens > 0 &&
      projected.totalTokens > this.#limits.maxTokens
    ) {
      throw new BudgetExceededError(
        "tokens",
        this.#limits.maxTokens,
        this.#totals.totalTokens,
        usage.totalTokens
      );
    }

    if (
      this.#limits.maxCostMicrousd != null &&
      this.#limits.maxCostMicrousd > 0 &&
      projected.costMicrousd > this.#limits.maxCostMicrousd
    ) {
      throw new BudgetExceededError(
        "cost",
        this.#limits.maxCostMicrousd,
        this.#totals.costMicrousd,
        usage.costMicrousd
      );
    }

    this.#totals = projected;
    return this.snapshot();
  }

  recordToolCall(): BudgetSnapshot {
    if (
      this.#limits.maxToolCalls != null &&
      this.#limits.maxToolCalls > 0 &&
      this.#toolCalls + 1 > this.#limits.maxToolCalls
    ) {
      throw new BudgetExceededError(
        "tool_calls",
        this.#limits.maxToolCalls,
        this.#toolCalls,
        1
      );
    }

    this.#toolCalls += 1;
    return this.snapshot();
  }

  recordIteration(): BudgetSnapshot {
    if (
      this.#limits.maxIterations != null &&
      this.#limits.maxIterations > 0 &&
      this.#iterations + 1 > this.#limits.maxIterations
    ) {
      throw new BudgetExceededError(
        "iterations",
        this.#limits.maxIterations,
        this.#iterations,
        1
      );
    }

    this.#iterations += 1;
    return this.snapshot();
  }

  assertWithinLimits(): BudgetSnapshot {
    if (
      this.#limits.maxTokens != null &&
      this.#limits.maxTokens > 0 &&
      this.#totals.totalTokens >= this.#limits.maxTokens
    ) {
      throw new BudgetExceededError(
        "tokens",
        this.#limits.maxTokens,
        this.#totals.totalTokens,
        0
      );
    }

    if (
      this.#limits.maxCostMicrousd != null &&
      this.#limits.maxCostMicrousd > 0 &&
      this.#totals.costMicrousd >= this.#limits.maxCostMicrousd
    ) {
      throw new BudgetExceededError(
        "cost",
        this.#limits.maxCostMicrousd,
        this.#totals.costMicrousd,
        0
      );
    }

    if (
      this.#limits.maxToolCalls != null &&
      this.#limits.maxToolCalls > 0 &&
      this.#toolCalls >= this.#limits.maxToolCalls
    ) {
      throw new BudgetExceededError(
        "tool_calls",
        this.#limits.maxToolCalls,
        this.#toolCalls,
        0
      );
    }

    if (
      this.#limits.maxIterations != null &&
      this.#limits.maxIterations > 0 &&
      this.#iterations >= this.#limits.maxIterations
    ) {
      throw new BudgetExceededError(
        "iterations",
        this.#limits.maxIterations,
        this.#iterations,
        0
      );
    }

    return this.snapshot();
  }

  snapshot(): BudgetSnapshot {
    return {
      ...this.#totals,
      toolCalls: this.#toolCalls,
      iterations: this.#iterations,
      limits: { ...this.#limits },
    };
  }
}
