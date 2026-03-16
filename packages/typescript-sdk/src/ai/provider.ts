import type { RunContext } from "../authoring/job";
import type { StraitProviderOptions } from "./types";

type LanguageModelV1Usage = {
  readonly promptTokens?: number;
  readonly completionTokens?: number;
};

type LanguageModelV1ToolCall = {
  readonly toolName: string;
  readonly args: unknown;
};

type GenerateResult = {
  readonly usage?: LanguageModelV1Usage;
  readonly toolCalls?: readonly LanguageModelV1ToolCall[];
  readonly [key: string]: unknown;
};

type StreamChunk = {
  readonly type: string;
  readonly textDelta?: string;
  readonly [key: string]: unknown;
};

type StreamResult = {
  readonly stream: ReadableStream<StreamChunk>;
  readonly usage?: LanguageModelV1Usage | Promise<LanguageModelV1Usage>;
  readonly [key: string]: unknown;
};

type DoGenerateParams = {
  readonly [key: string]: unknown;
};

type DoStreamParams = {
  readonly [key: string]: unknown;
};

type DoGenerateFn = (params: DoGenerateParams) => PromiseLike<GenerateResult>;
type DoStreamFn = (params: DoStreamParams) => PromiseLike<StreamResult>;

export type LanguageModelV1Middleware = {
  readonly wrapGenerate?: (options: {
    readonly doGenerate: DoGenerateFn;
    readonly params: DoGenerateParams;
  }) => PromiseLike<GenerateResult>;
  readonly wrapStream?: (options: {
    readonly doStream: DoStreamFn;
    readonly params: DoStreamParams;
  }) => PromiseLike<StreamResult>;
};

const fireAndForget = (promise: Promise<unknown>) => {
  promise.catch(() => undefined);
};

export const createStraitProvider = (
  ctx: RunContext,
  options?: StraitProviderOptions
): LanguageModelV1Middleware => {
  const reportUsage = options?.reportUsage ?? true;
  const logToolCalls = options?.logToolCalls ?? true;
  const streamToStrait = options?.streamToStrait ?? true;
  const providerName = options?.providerName ?? "unknown";

  return {
    wrapGenerate: async ({ doGenerate, params }) => {
      const result = await doGenerate(params);

      if (reportUsage && result.usage && ctx.reportUsage) {
        const totalTokens =
          (result.usage.promptTokens ?? 0) +
          (result.usage.completionTokens ?? 0);
        await ctx.reportUsage({
          provider: providerName,
          model: providerName,
          promptTokens: result.usage.promptTokens,
          completionTokens: result.usage.completionTokens,
          totalTokens,
        });
      }

      if (logToolCalls && result.toolCalls && ctx.logToolCall) {
        for (const tc of result.toolCalls) {
          await ctx.logToolCall({
            toolName: tc.toolName,
            input:
              typeof tc.args === "object" && tc.args !== null
                ? (tc.args as Record<string, unknown>)
                : undefined,
          });
        }
      }

      return result;
    },

    wrapStream: async ({ doStream, params }) => {
      const result = await doStream(params);

      if (!(streamToStrait && ctx.streamChunk)) {
        return result;
      }

      const streamChunkFn = ctx.streamChunk;
      const originalStream = result.stream;

      const transformedStream = originalStream.pipeThrough(
        new TransformStream<StreamChunk, StreamChunk>({
          transform(chunk, controller) {
            if (chunk.type === "text-delta" && chunk.textDelta) {
              fireAndForget(streamChunkFn(chunk.textDelta));
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
