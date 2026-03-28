import { generateText, streamText } from "ai";

import type { StraitContext } from "./context";
import type { JsonValue } from "./types";

type AdapterContext = Pick<
  StraitContext,
  "checkpoint" | "reportToolCall" | "reportUsage" | "stream"
>;

type GenerateTextOptions = Parameters<typeof generateText>[0];
type StreamTextOptions = Parameters<typeof streamText>[0];
type GenerateTextResult = Awaited<ReturnType<typeof generateText>>;
type StreamTextResult = ReturnType<typeof streamText>;

type OnStepFinishEvent =
  NonNullable<GenerateTextOptions["onStepFinish"]> extends (
    event: infer Event
  ) => unknown
    ? Event
    : never;

type OnToolCallFinishEvent =
  NonNullable<GenerateTextOptions["experimental_onToolCallFinish"]> extends (
    event: infer Event
  ) => unknown
    ? Event
    : never;

type OnFinishEvent =
  NonNullable<GenerateTextOptions["onFinish"]> extends (
    event: infer Event
  ) => unknown
    ? Event
    : never;

type StreamChunkEvent =
  NonNullable<StreamTextOptions["onChunk"]> extends (
    event: infer Event
  ) => unknown
    ? Event
    : never;

export interface VercelAIAdapterOptions {
  checkpoint?: {
    source?: string;
    onStepFinish?: (
      event: OnStepFinishEvent
    ) => JsonValue | undefined | PromiseLike<JsonValue | undefined>;
  };
  streamId?: string;
}

function toJSONValue(value: unknown): JsonValue {
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

async function reportUsageForStep(
  context: AdapterContext,
  event: OnStepFinishEvent
): Promise<void> {
  await context.reportUsage({
    provider: event.model.provider,
    model: event.model.modelId,
    promptTokens: event.usage.inputTokens ?? 0,
    completionTokens: event.usage.outputTokens ?? 0,
    totalTokens:
      event.usage.totalTokens ??
      (event.usage.inputTokens ?? 0) + (event.usage.outputTokens ?? 0),
  });
}

async function reportCheckpointForStep(
  context: AdapterContext,
  adapterOptions: VercelAIAdapterOptions | undefined,
  event: OnStepFinishEvent
): Promise<void> {
  const checkpointState =
    await adapterOptions?.checkpoint?.onStepFinish?.(event);
  if (checkpointState === undefined) {
    return;
  }

  await context.checkpoint(checkpointState, {
    source: adapterOptions?.checkpoint?.source,
  });
}

async function reportToolCall(
  context: AdapterContext,
  event: OnToolCallFinishEvent
): Promise<void> {
  await context.reportToolCall({
    toolName: event.toolCall.toolName,
    input: toJSONValue(event.toolCall.input),
    output: event.success
      ? toJSONValue(event.output)
      : toJSONValue(event.error),
    durationMs: Math.round(event.durationMs),
    status: event.success ? "completed" : "failed",
  });
}

async function reportStreamChunk(
  context: AdapterContext,
  adapterOptions: VercelAIAdapterOptions | undefined,
  event: StreamChunkEvent
): Promise<void> {
  const chunk = event.chunk;
  if (chunk.type !== "text-delta" && chunk.type !== "reasoning-delta") {
    return;
  }

  await context.stream({
    chunk: chunk.text,
    streamId: adapterOptions?.streamId,
  });
}

async function reportStreamDone(
  context: AdapterContext,
  adapterOptions: VercelAIAdapterOptions | undefined
): Promise<void> {
  await context.stream({
    chunk: "",
    streamId: adapterOptions?.streamId,
    done: true,
  });
}

export function createVercelAIAdapter(
  context: AdapterContext,
  adapterOptions?: VercelAIAdapterOptions
): {
  generateText: (options: GenerateTextOptions) => Promise<GenerateTextResult>;
  streamText: (options: StreamTextOptions) => StreamTextResult;
} {
  return {
    generateText(options) {
      return generateText({
        ...options,
        experimental_onToolCallFinish: async (event) => {
          await reportToolCall(context, event);
          await options.experimental_onToolCallFinish?.(event);
        },
        onStepFinish: async (event) => {
          await reportUsageForStep(context, event);
          await reportCheckpointForStep(context, adapterOptions, event);
          await options.onStepFinish?.(event);
        },
        onFinish: async (event) => {
          await options.onFinish?.(event as OnFinishEvent);
        },
      });
    },
    streamText(options) {
      return streamText({
        ...options,
        experimental_onToolCallFinish: async (event) => {
          await reportToolCall(context, event);
          await options.experimental_onToolCallFinish?.(event);
        },
        onChunk: async (event) => {
          await reportStreamChunk(context, adapterOptions, event);
          await options.onChunk?.(event);
        },
        onStepFinish: async (event) => {
          await reportUsageForStep(context, event);
          await reportCheckpointForStep(context, adapterOptions, event);
          await options.onStepFinish?.(event);
        },
        onFinish: async (event) => {
          await reportStreamDone(context, adapterOptions);
          await options.onFinish?.(event as OnFinishEvent);
        },
      });
    },
  };
}
