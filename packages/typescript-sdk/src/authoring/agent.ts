import type { RunContext } from "./job";
import { type DefineJobOptions, defineJob } from "./job";
import type { SchemaInput } from "./types";

export type AgentRunContext = RunContext & {
  readonly iteration: number;
  readonly accumulatedCostMicrousd: () => number;
  readonly isBudgetExceeded: () => boolean;
};

export type DefineAgentOptions<TPayload, TOutput = unknown> = {
  readonly name: string;
  readonly slug: string;
  readonly endpointUrl: string;
  readonly schema: SchemaInput<TPayload>;
  readonly projectId?: string;
  readonly maxIterations?: number;
  readonly maxCostMicrousd?: number;
  readonly autoCheckpoint?: boolean;
  readonly providerName?: string;
  readonly run: (
    payload: TPayload,
    ctx: AgentRunContext
  ) => Promise<TOutput> | TOutput;
  readonly description?: string;
  readonly tags?: Readonly<Record<string, string>>;
  readonly timeoutSecs?: number;
  readonly maxAttempts?: number;
  readonly retryStrategy?: "exponential" | "fixed";
  readonly onSuccess?: DefineJobOptions<TPayload, TOutput>["onSuccess"];
  readonly onFailure?: DefineJobOptions<TPayload, TOutput>["onFailure"];
  readonly onStart?: DefineJobOptions<TPayload, TOutput>["onStart"];
};

export const defineAgent = <TPayload, TOutput = unknown>(
  options: DefineAgentOptions<TPayload, TOutput>
) => {
  const maxCost = options.maxCostMicrousd ?? Number.POSITIVE_INFINITY;
  const autoCheckpoint = options.autoCheckpoint ?? true;

  return defineJob<TPayload, TOutput>({
    name: options.name,
    slug: options.slug,
    endpointUrl: options.endpointUrl,
    schema: options.schema,
    projectId: options.projectId,
    description: options.description,
    tags: { ...options.tags, "strait.kind": "agent" },
    timeoutSecs: options.timeoutSecs ?? 600,
    maxAttempts: options.maxAttempts ?? 5,
    retryStrategy: options.retryStrategy ?? "exponential",
    onSuccess: options.onSuccess,
    onFailure: options.onFailure,
    onStart: options.onStart,
    run: (payload, ctx) => {
      let accumulatedCost = 0;
      let iteration = 0;

      const agentCtx: AgentRunContext = {
        ...ctx,
        get iteration() {
          return iteration;
        },
        accumulatedCostMicrousd: () => accumulatedCost,
        isBudgetExceeded: () => accumulatedCost >= maxCost,
        reportUsage: ctx.reportUsage
          ? async (usage) => {
              if (usage.costMicrousd) {
                accumulatedCost += usage.costMicrousd;
              }
              await ctx.reportUsage?.(usage);
            }
          : undefined,
        checkpoint: async (state) => {
          iteration++;
          if (autoCheckpoint) {
            await ctx.checkpoint(state);
          }
        },
      };

      return options.run(payload, agentCtx);
    },
  });
};
