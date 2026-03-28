import { Effect } from "effect";

import { StraitContext } from "./context";
import { runPromise } from "./effects";
import { StraitSDKError } from "./errors";
import {
  defaultPricingCatalog,
  estimateUsageCostMicrousd,
  normalizeUsageReport,
} from "./pricing";
import type {
  AgentBudget,
  BudgetInput,
  CheckpointOptions,
  JsonValue,
  PricingCatalog,
  StreamChunkReport,
  ToolCallReport,
  UsageReport,
  UsageTotals,
} from "./types";

export interface AdapterContext {
  budgetExceeded?: () => void;
  budgetSnapshot?: StraitContext["budgetSnapshot"];
  checkpoint?: StraitContext["checkpoint"];
  reportToolCall: StraitContext["reportToolCall"];
  reportUsage: StraitContext["reportUsage"];
  stream: StraitContext["stream"];
}

export interface AdapterTelemetryOptions {
  budget?: BudgetInput;
  checkpoint?: {
    onStepFinish?: (
      event: unknown
    ) => JsonValue | undefined | PromiseLike<JsonValue | undefined>;
    source?: string;
  };
  context?: AdapterContext;
  pricingCatalog?: PricingCatalog;
  streamId?: string;
}

export type AdapterContextInput = AdapterContext;

const MICROS_PER_USD = 1_000_000;

function requireNonEmpty(value: string, field: string): string {
  const normalized = value.trim();
  if (normalized.length === 0) {
    throw new StraitSDKError(`${field} is required`);
  }
  return normalized;
}

function parseBudgetAmount(value: string): number {
  const trimmed = requireNonEmpty(value, "budget");
  const normalized = trimmed.startsWith("$") ? trimmed.slice(1) : trimmed;
  const numeric = Number(normalized);
  if (!Number.isFinite(numeric) || numeric < 0) {
    throw new StraitSDKError("budget must be a positive USD amount");
  }
  return Math.round(numeric * MICROS_PER_USD);
}

export function normalizeBudgetInput(
  input?: BudgetInput
): AgentBudget | undefined {
  if (input == null) {
    return undefined;
  }

  if (typeof input === "string") {
    return {
      maxCostMicrousd: parseBudgetAmount(input),
    };
  }

  if (typeof input === "number") {
    if (!Number.isFinite(input) || input < 0) {
      throw new StraitSDKError("budget must be a positive USD amount");
    }
    return {
      maxCostMicrousd: Math.round(input * MICROS_PER_USD),
    };
  }

  return { ...input };
}

export function createAdapterContext(
  context: AdapterContextInput
): AdapterContext {
  return {
    ...context,
    budgetExceeded: context.budgetExceeded ?? (() => undefined),
    budgetSnapshot:
      context.budgetSnapshot ??
      (() => ({
        promptTokens: 0,
        completionTokens: 0,
        totalTokens: 0,
        costMicrousd: 0,
        toolCalls: 0,
        limits: {},
      })),
    checkpoint: context.checkpoint ?? (() => Promise.resolve({} as never)),
  };
}

export function resolveAdapterContext(
  options: AdapterTelemetryOptions = {}
): AdapterContext {
  if (options.context != null) {
    return createAdapterContext(options.context);
  }

  return createAdapterContext(
    StraitContext.fromEnv(undefined, {
      budget: normalizeBudgetInput(options.budget),
      pricingCatalog: options.pricingCatalog,
    })
  );
}

export function toJsonValue(value: unknown): JsonValue {
  if (
    value === null ||
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  ) {
    return value;
  }

  if (value == null) {
    return null;
  }

  if (value instanceof Error) {
    return {
      name: value.name,
      message: value.message,
    };
  }

  try {
    return JSON.parse(JSON.stringify(value)) as JsonValue;
  } catch {
    return {
      value: String(value),
    };
  }
}

export function parseJsonish(value: string): JsonValue {
  try {
    return JSON.parse(value) as JsonValue;
  } catch {
    return value;
  }
}

export function reportSafely<T>(operation: () => Promise<T>): Promise<void> {
  return runPromise(
    Effect.tryPromise({
      try: operation,
      catch: () => undefined,
    }).pipe(Effect.catchAll(() => Effect.void))
  ).then(() => undefined);
}

export function budgetGuard(context: AdapterContext): void {
  context.budgetExceeded?.();
}

export function reportCheckpointState(
  context: AdapterContext,
  checkpointOptions: AdapterTelemetryOptions["checkpoint"],
  event: unknown
): Promise<void> {
  return reportSafely(async () => {
    if (context.checkpoint == null) {
      return;
    }

    const state = await checkpointOptions?.onStepFinish?.(event);
    if (state === undefined) {
      return;
    }

    await context.checkpoint(state, {
      source: checkpointOptions?.source,
    } satisfies CheckpointOptions);
  });
}

export function reportStreamChunk(
  context: AdapterContext,
  report: StreamChunkReport
): Promise<void> {
  return reportSafely(() => context.stream(report));
}

export function reportToolEvent(
  context: AdapterContext,
  report: ToolCallReport
): Promise<void> {
  return reportSafely(() => context.reportToolCall(report));
}

export function reportUsageEvent(
  context: AdapterContext,
  usage: UsageReport
): Promise<void> {
  return reportSafely(() => context.reportUsage(usage));
}

export function sumUsageTotals(
  totals: UsageTotals,
  usage: UsageReport,
  pricingCatalog: PricingCatalog = defaultPricingCatalog
): UsageTotals {
  const normalized = normalizeUsageReport(usage, pricingCatalog);

  return {
    promptTokens: totals.promptTokens + normalized.promptTokens,
    completionTokens: totals.completionTokens + normalized.completionTokens,
    totalTokens: totals.totalTokens + normalized.totalTokens,
    costMicrousd: totals.costMicrousd + normalized.costMicrousd,
  };
}

export function estimateCostFromUsage(
  usage: Pick<
    UsageReport,
    "completionTokens" | "model" | "promptTokens" | "provider"
  >,
  pricingCatalog: PricingCatalog = defaultPricingCatalog
): number {
  return (
    estimateUsageCostMicrousd(usage, pricingCatalog) ??
    normalizeUsageReport(usage, pricingCatalog).costMicrousd
  );
}
