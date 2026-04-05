import { Effect, Either } from "effect";

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
  recordIteration?: () => void;
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
        iterations: 0,
        limits: {},
      })),
    checkpoint: context.checkpoint ?? (() => Promise.resolve({} as never)),
    recordIteration: context.recordIteration ?? undefined,
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

/** Safely converts an unknown value to a JSON-serializable form. */
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

  const result = Either.try(
    () => JSON.parse(JSON.stringify(value)) as JsonValue
  );
  return Either.isRight(result) ? result.right : { value: String(value) };
}

/** Attempts to parse a string as JSON, falling back to the raw string on failure. */
export function parseJsonish(value: string): JsonValue {
  const result = Either.try(() => JSON.parse(value) as JsonValue);
  return Either.isRight(result) ? result.right : value;
}

/** Executes a reporting operation, suppressing errors to avoid interrupting agent execution. */
export function reportSafely<T>(operation: () => Promise<T>): Promise<void> {
  return runPromise(
    Effect.tryPromise({
      try: operation,
      catch: (e) =>
        e instanceof StraitSDKError
          ? e
          : new StraitSDKError("report failed", { cause: e }),
    }).pipe(
      Effect.catchIf(
        (e): e is StraitSDKError => e instanceof StraitSDKError,
        () => Effect.void
      )
    )
  ).then(() => undefined);
}

/** Throws BudgetExceededError if the adapter's budget limit has been reached. */
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

    // Enrich checkpoint with budget snapshot for per-step cost attribution.
    let enrichedState = state;
    if (
      context.budgetSnapshot != null &&
      typeof state === "object" &&
      state !== null &&
      !Array.isArray(state)
    ) {
      const snapshot = context.budgetSnapshot();
      enrichedState = {
        ...(state as Record<string, JsonValue>),
        cost_microusd_at_checkpoint: snapshot.costMicrousd,
        tokens_at_checkpoint: snapshot.totalTokens,
        iterations_at_checkpoint: snapshot.iterations,
      };
    }

    await context.checkpoint(enrichedState, {
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

/** Accumulates usage totals from a new usage report, computing cost via the pricing catalog. */
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
