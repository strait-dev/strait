import type { LanguageModelMiddleware } from "ai";
import type { RunContext } from "../authoring/job";
import type { StraitProviderOptions } from "./types";

/** AI SDK v6 language model middleware, extended to avoid tsgo portability issues. */
export interface StraitMiddleware extends LanguageModelMiddleware {}

const fireAndForget = (promise: Promise<unknown>) => {
  promise.catch(() => undefined);
};

type V3Usage = {
  readonly inputTokens?: { readonly total?: number };
  readonly outputTokens?: { readonly total?: number };
};

type V3ContentItem = {
  readonly type: string;
  readonly toolName?: string;
  readonly input?: string;
};

const handleUsageReport = async (
  usage: V3Usage,
  providerName: string,
  ctx: RunContext
) => {
  const promptTokens = usage.inputTokens?.total ?? 0;
  const completionTokens = usage.outputTokens?.total ?? 0;
  await ctx.reportUsage?.({
    provider: providerName,
    model: providerName,
    promptTokens,
    completionTokens,
    totalTokens: promptTokens + completionTokens,
  });
};

const parseToolInput = (raw: string): Record<string, unknown> | undefined => {
  try {
    return JSON.parse(raw);
  } catch {
    return undefined;
  }
};

const handleToolCallLogging = async (
  content: readonly V3ContentItem[],
  ctx: RunContext
) => {
  for (const item of content) {
    if (item.type === "tool-call" && item.toolName) {
      const parsed =
        typeof item.input === "string" ? parseToolInput(item.input) : undefined;
      await ctx.logToolCall?.({
        toolName: item.toolName,
        input: parsed,
      });
    }
  }
};

export const createStraitProvider = (
  ctx: RunContext,
  options?: StraitProviderOptions
): StraitMiddleware => {
  const reportUsage = options?.reportUsage ?? true;
  const logToolCalls = options?.logToolCalls ?? true;
  const streamToStrait = options?.streamToStrait ?? true;
  const providerName = options?.providerName ?? "unknown";

  return {
    specificationVersion: "v3",

    wrapGenerate: async ({ doGenerate }) => {
      const result = await doGenerate();

      if (reportUsage && result.usage && ctx.reportUsage) {
        await handleUsageReport(result.usage, providerName, ctx);
      }

      if (logToolCalls && result.content && ctx.logToolCall) {
        await handleToolCallLogging(result.content, ctx);
      }

      return result;
    },

    wrapStream: async ({ doStream }) => {
      const result = await doStream();

      if (!(streamToStrait && ctx.streamChunk)) {
        return result;
      }

      const streamChunkFn = ctx.streamChunk;
      const originalStream = result.stream;

      const transformedStream = originalStream.pipeThrough(
        new TransformStream({
          transform(chunk, controller) {
            if (chunk.type === "text-delta" && "delta" in chunk) {
              fireAndForget(streamChunkFn(chunk.delta as string));
            }
            controller.enqueue(chunk);
          },
          flush() {
            fireAndForget(streamChunkFn("", { done: true }));
          },
        })
      );

      return { ...result, stream: transformedStream };
    },
  };
};
