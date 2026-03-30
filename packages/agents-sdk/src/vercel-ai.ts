import {
  customProvider,
  generateText,
  type LanguageModelMiddleware,
  type OnStepFinishEvent,
  type OnToolCallFinishEvent,
  type PrepareStepFunction,
  type Provider,
  type StopCondition,
  streamText,
  type TelemetryIntegration,
  wrapLanguageModel,
} from "ai";
import type { AdapterTelemetryOptions } from "./internal";
import {
  budgetGuard,
  normalizeBudgetInput,
  reportCheckpointState,
  reportStreamChunk,
  reportToolEvent,
  reportUsageEvent,
  resolveAdapterContext,
  sumUsageTotals,
  toJsonValue,
} from "./internal";
import { defaultPricingCatalog } from "./pricing";
import { applyPromptCaching, type PromptCacheOptions } from "./prompt-cache";
import type { AgentBudget, BudgetInput, UsageTotals } from "./types";

type AdapterContext = ReturnType<typeof resolveAdapterContext>;
type VercelLanguageModel = Parameters<typeof wrapLanguageModel>[0]["model"];

type GenerateTextOptions = Parameters<typeof generateText>[0];
type StreamTextOptions = Parameters<typeof streamText>[0];
type GenerateTextResult = Awaited<ReturnType<typeof generateText>>;
type StreamTextResult = ReturnType<typeof streamText>;

type StreamChunkEvent =
  NonNullable<StreamTextOptions["onChunk"]> extends (
    event: infer Event
  ) => unknown
    ? Event
    : never;

export interface VercelAIAdapterOptions extends AdapterTelemetryOptions {
  /** Enable prompt caching for compatible providers (Anthropic, OpenAI). */
  promptCaching?: PromptCacheOptions;
}

export interface AutoBudgetOptions {
  above: `${number}%` | number;
  budget: BudgetInput;
  disableTools?: string[];
  switchTo: VercelLanguageModel;
}

function toUsageReport(event: OnStepFinishEvent<any>) {
  return {
    provider: event.model.provider,
    model: event.model.modelId,
    promptTokens: event.usage.inputTokens ?? 0,
    completionTokens: event.usage.outputTokens ?? 0,
    totalTokens:
      event.usage.totalTokens ??
      (event.usage.inputTokens ?? 0) + (event.usage.outputTokens ?? 0),
    promptTokenDetails: {
      cacheReadTokens: event.usage.inputTokenDetails?.cacheReadTokens,
      cacheWriteTokens: event.usage.inputTokenDetails?.cacheWriteTokens,
    },
    completionTokenDetails: {
      reasoningTokens: event.usage.outputTokenDetails?.reasoningTokens,
      textTokens: event.usage.outputTokenDetails?.textTokens,
    },
  } as const;
}

function toPercentThreshold(value: `${number}%` | number): number {
  if (typeof value === "number") {
    return value > 1 ? value / 100 : value;
  }

  return Number.parseFloat(value.slice(0, -1)) / 100;
}

function extractBudgetLimit(input: BudgetInput): AgentBudget {
  return normalizeBudgetInput(input) ?? {};
}

function mergeTelemetryIntegrations(
  integrations: TelemetryIntegration | TelemetryIntegration[] | undefined,
  telemetry: TelemetryIntegration
): TelemetryIntegration[] {
  if (Array.isArray(integrations)) {
    return [telemetry, ...integrations];
  }

  if (integrations != null) {
    return [telemetry, integrations];
  }

  return [telemetry];
}

function coerceUsageNumber(value: unknown): number {
  if (typeof value === "number") {
    return value;
  }

  if (
    value != null &&
    typeof value === "object" &&
    "total" in value &&
    typeof (value as { total?: unknown }).total === "number"
  ) {
    return (value as { total: number }).total;
  }

  return 0;
}

function usageToReport(
  usage: Record<string, unknown> | undefined,
  model: { modelId: string; provider: string }
) {
  const promptTokenDetails = usage?.inputTokenDetails as
    | {
        cacheReadTokens?: number;
        cacheWriteTokens?: number;
      }
    | undefined;
  const completionTokenDetails = usage?.outputTokenDetails as
    | {
        reasoningTokens?: number;
        textTokens?: number;
      }
    | undefined;

  const promptTokens = coerceUsageNumber(usage?.inputTokens);
  const completionTokens = coerceUsageNumber(usage?.outputTokens);

  return {
    provider: model.provider,
    model: model.modelId,
    promptTokens,
    completionTokens,
    totalTokens:
      typeof usage?.totalTokens === "number"
        ? usage.totalTokens
        : promptTokens + completionTokens,
    promptTokenDetails: {
      cacheReadTokens: promptTokenDetails?.cacheReadTokens,
      cacheWriteTokens: promptTokenDetails?.cacheWriteTokens,
    },
    completionTokenDetails: {
      reasoningTokens: completionTokenDetails?.reasoningTokens,
      textTokens: completionTokenDetails?.textTokens,
    },
  } as const;
}

function buildMiddleware(
  context: AdapterContext,
  options: VercelAIAdapterOptions
): LanguageModelMiddleware {
  return {
    specificationVersion: "v3",
    transformParams: async ({ params }) => {
      budgetGuard(context);
      const transformed = options.promptCaching
        ? applyPromptCaching(params, options.promptCaching)
        : params;
      return await Promise.resolve(transformed);
    },
    wrapGenerate: async ({ doGenerate, params, model }) => {
      const result = await doGenerate();
      const usage = (result as { usage?: Record<string, unknown> }).usage;
      if (usage != null) {
        await reportUsageEvent(context, {
          ...usageToReport(usage, model),
          metadata: toJsonValue({
            providerOptions: params.providerOptions,
          }),
        });
      }

      return result;
    },
    wrapStream: async ({ doStream, model }) => {
      const result = await doStream();
      const stream = result.stream.pipeThrough(
        new TransformStream({
          async transform(chunk: any, controller) {
            if (chunk.type === "text-delta") {
              await reportStreamChunk(context, {
                chunk: chunk.delta,
                streamId: options.streamId,
              });
            }

            if (chunk.type === "reasoning-delta") {
              await reportStreamChunk(context, {
                chunk: chunk.delta,
                streamId: options.streamId,
              });
            }

            if (chunk.type === "finish") {
              await Promise.all([
                reportUsageEvent(context, usageToReport(chunk.usage, model)),
                reportStreamChunk(context, {
                  chunk: "",
                  streamId: options.streamId,
                  done: true,
                }),
              ]);
            }

            controller.enqueue(chunk);
          },
        })
      );

      return {
        ...result,
        stream,
      };
    },
  } as LanguageModelMiddleware;
}

async function reportToolCall(
  context: AdapterContext,
  event: OnToolCallFinishEvent<any>
): Promise<void> {
  await reportToolEvent(context, {
    toolName: event.toolCall.toolName,
    input: toJsonValue(event.toolCall.input),
    output: event.success
      ? toJsonValue(event.output)
      : toJsonValue(event.error),
    durationMs: Math.round(event.durationMs),
    status: event.success ? "completed" : "failed",
  });
}

async function reportStep(
  context: AdapterContext,
  options: VercelAIAdapterOptions,
  event: OnStepFinishEvent<any>
): Promise<void> {
  await Promise.all([
    reportUsageEvent(context, toUsageReport(event)),
    reportCheckpointState(context, options.checkpoint, event),
  ]);
}

export function straitTelemetry(
  options: VercelAIAdapterOptions = {}
): TelemetryIntegration {
  const context = resolveAdapterContext(options);

  return {
    onToolCallFinish: async (event) => {
      try {
        await reportToolCall(context, event);
      } catch {
        return;
      }
    },
    onStepFinish: async (event) => {
      try {
        await reportStep(context, options, event);
      } catch {
        return;
      }
    },
    onFinish: async () => {
      try {
        await reportStreamChunk(context, {
          chunk: "",
          streamId: options.streamId,
          done: true,
        });
      } catch {
        return;
      }
    },
  };
}

export function budgetExceeded(
  limit: BudgetInput,
  options: Pick<VercelAIAdapterOptions, "pricingCatalog"> = {}
): StopCondition<any> {
  const pricingCatalog = options.pricingCatalog ?? defaultPricingCatalog;
  const budget = extractBudgetLimit(limit);

  return ({ steps }) => {
    const totals = steps.reduce<UsageTotals>(
      (current, step) =>
        sumUsageTotals(
          current,
          {
            provider: step.model.provider,
            model: step.model.modelId,
            promptTokens: step.usage.inputTokens ?? 0,
            completionTokens: step.usage.outputTokens ?? 0,
            totalTokens:
              step.usage.totalTokens ??
              (step.usage.inputTokens ?? 0) + (step.usage.outputTokens ?? 0),
          },
          pricingCatalog
        ),
      {
        promptTokens: 0,
        completionTokens: 0,
        totalTokens: 0,
        costMicrousd: 0,
      }
    );

    if (
      budget.maxCostMicrousd != null &&
      budget.maxCostMicrousd > 0 &&
      totals.costMicrousd >= budget.maxCostMicrousd
    ) {
      return true;
    }

    if (
      budget.maxTokens != null &&
      budget.maxTokens > 0 &&
      totals.totalTokens >= budget.maxTokens
    ) {
      return true;
    }

    return false;
  };
}

export function autoBudget(
  options: AutoBudgetOptions
): PrepareStepFunction<any> {
  const limit = extractBudgetLimit(options.budget);
  const threshold = toPercentThreshold(options.above);

  return (input: any) => {
    const { model, steps } = input;
    if (limit.maxCostMicrousd == null || limit.maxCostMicrousd <= 0) {
      return undefined;
    }

    const totals = (steps as any[]).reduce(
      (current: UsageTotals, step: any) =>
        sumUsageTotals(current, {
          provider: step.model.provider,
          model: step.model.modelId,
          promptTokens: step.usage.inputTokens ?? 0,
          completionTokens: step.usage.outputTokens ?? 0,
          totalTokens:
            step.usage.totalTokens ??
            (step.usage.inputTokens ?? 0) + (step.usage.outputTokens ?? 0),
        }),
      {
        promptTokens: 0,
        completionTokens: 0,
        totalTokens: 0,
        costMicrousd: 0,
      } satisfies UsageTotals
    );

    if (totals.costMicrousd < limit.maxCostMicrousd * threshold) {
      return undefined;
    }

    return {
      model: options.switchTo ?? model,
      activeTools:
        options.disableTools == null || input.activeTools == null
          ? input.activeTools
          : (input.activeTools as string[]).filter(
              (tool: string) => !options.disableTools?.includes(tool)
            ),
    };
  };
}

export function provider(
  fallback: Provider,
  options: VercelAIAdapterOptions = {}
): Provider {
  return customProvider({
    fallbackProvider: {
      specificationVersion: "v3",
      ...(fallback as any),
      languageModel(modelId) {
        return withStrait((fallback as any).languageModel(modelId), options);
      },
    },
  }) as Provider;
}

export function withStrait(
  model: VercelLanguageModel,
  options: VercelAIAdapterOptions = {}
): VercelLanguageModel {
  const context = resolveAdapterContext(options);

  return wrapLanguageModel({
    model,
    middleware: buildMiddleware(context, options),
  });
}

export function createVercelAIAdapter(
  context: AdapterContext,
  adapterOptions?: VercelAIAdapterOptions
): {
  generateText: (options: GenerateTextOptions) => Promise<GenerateTextResult>;
  streamText: (options: StreamTextOptions) => StreamTextResult;
};
export function createVercelAIAdapter(options: VercelAIAdapterOptions): {
  generateText: (options: GenerateTextOptions) => Promise<GenerateTextResult>;
  streamText: (options: StreamTextOptions) => StreamTextResult;
};
export function createVercelAIAdapter(
  contextOrOptions: AdapterContext | VercelAIAdapterOptions,
  adapterOptions?: VercelAIAdapterOptions
): {
  generateText: (options: GenerateTextOptions) => Promise<GenerateTextResult>;
  streamText: (options: StreamTextOptions) => StreamTextResult;
} {
  const options =
    "reportUsage" in contextOrOptions
      ? {
          ...adapterOptions,
          context: contextOrOptions,
        }
      : contextOrOptions;

  const context = resolveAdapterContext(options);
  const telemetry = straitTelemetry({
    ...options,
    context,
  });

  return {
    generateText(optionsArg) {
      budgetGuard(context);

      return generateText({
        ...optionsArg,
        experimental_telemetry: {
          ...optionsArg.experimental_telemetry,
          integrations: mergeTelemetryIntegrations(
            optionsArg.experimental_telemetry?.integrations,
            telemetry
          ),
        },
        experimental_onToolCallFinish: async (event) => {
          await reportToolCall(context, event);
          await optionsArg.experimental_onToolCallFinish?.(event);
        },
        onStepFinish: async (event) => {
          await reportStep(context, options, event);
          await optionsArg.onStepFinish?.(event);
        },
        onFinish: async (event) => {
          await reportStreamChunk(context, {
            chunk: "",
            streamId: options.streamId,
            done: true,
          });
          await optionsArg.onFinish?.(event);
        },
      });
    },
    streamText(optionsArg) {
      budgetGuard(context);

      return streamText({
        ...optionsArg,
        experimental_telemetry: {
          ...optionsArg.experimental_telemetry,
          integrations: mergeTelemetryIntegrations(
            optionsArg.experimental_telemetry?.integrations,
            telemetry
          ),
        },
        experimental_onToolCallFinish: async (event) => {
          await reportToolCall(context, event);
          await optionsArg.experimental_onToolCallFinish?.(event);
        },
        onChunk: async (event: StreamChunkEvent) => {
          const chunk = event.chunk;
          if (chunk.type === "text-delta" || chunk.type === "reasoning-delta") {
            await reportStreamChunk(context, {
              chunk: (chunk as { text: string }).text,
              streamId: options.streamId,
            });
          }
          await optionsArg.onChunk?.(event);
        },
        onStepFinish: async (event) => {
          await reportStep(context, options, event);
          await optionsArg.onStepFinish?.(event);
        },
        onFinish: async (event) => {
          await reportStreamChunk(context, {
            chunk: "",
            streamId: options.streamId,
            done: true,
          });
          await optionsArg.onFinish?.(event);
        },
      });
    },
  };
}
