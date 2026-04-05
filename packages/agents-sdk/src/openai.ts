import type { AdapterContextInput, AdapterTelemetryOptions } from "./internal";
import {
  budgetGuard,
  createAdapterContext,
  parseJsonish,
  reportCheckpointState,
  reportStreamChunk,
  reportToolEvent,
  reportUsageEvent,
  resolveAdapterContext,
} from "./internal";

type OpenAIUsage = {
  completion_tokens: number;
  prompt_tokens: number;
  prompt_tokens_details?: {
    cached_tokens?: number;
  };
  total_tokens: number;
};

type OpenAIChatCompletion = {
  model: string;
  usage?: OpenAIUsage;
};

type OpenAIToolCall = {
  arguments: string;
  name: string;
};

type OpenAIEventStreamLike = {
  on(event: string, listener: (...args: unknown[]) => unknown): unknown;
};

type OpenAICompletionsLike = {
  create: (...args: unknown[]) => PromiseLike<OpenAIChatCompletion>;
  runTools: (...args: unknown[]) => OpenAIEventStreamLike;
  stream: (...args: unknown[]) => OpenAIEventStreamLike;
};

type OpenAIResponsesLike = {
  create: (...args: unknown[]) => PromiseLike<{
    model?: string;
    usage?: OpenAIUsage;
  }>;
};

type OpenAIThreadsLike = Record<PropertyKey, unknown>;

type OpenAIClientLike = {
  beta?: {
    threads?: OpenAIThreadsLike;
  };
  chat: {
    completions: OpenAICompletionsLike;
  };
  responses?: OpenAIResponsesLike;
} & Record<PropertyKey, unknown>;

export interface OpenAIAdapterOptions extends AdapterTelemetryOptions {}

type PendingToolCall = {
  arguments: string;
  name: string;
  startedAt: number;
};

async function reportUsage(
  context: ReturnType<typeof resolveAdapterContext>,
  completion: OpenAIChatCompletion
): Promise<void> {
  if (completion.usage == null) {
    return;
  }

  await reportUsageEvent(context, {
    provider: "openai",
    model: completion.model,
    promptTokens: completion.usage.prompt_tokens,
    completionTokens: completion.usage.completion_tokens,
    totalTokens: completion.usage.total_tokens,
    ...(completion.usage.prompt_tokens_details?.cached_tokens == null
      ? {}
      : {
          promptTokenDetails: {
            cacheReadTokens:
              completion.usage.prompt_tokens_details.cached_tokens,
          },
        }),
  });
}

function wireStream(
  context: ReturnType<typeof resolveAdapterContext>,
  runner: OpenAIEventStreamLike,
  options: OpenAIAdapterOptions
): OpenAIEventStreamLike {
  runner.on("content", (delta) => {
    if (typeof delta !== "string") {
      return;
    }

    return reportStreamChunk(context, {
      chunk: delta,
      streamId: options.streamId,
    });
  });

  runner.on("finalChatCompletion", (completion) => {
    if (
      completion == null ||
      typeof completion !== "object" ||
      typeof (completion as OpenAIChatCompletion).model !== "string"
    ) {
      return;
    }

    return Promise.all([
      reportUsage(context, completion as OpenAIChatCompletion),
      reportStreamChunk(context, {
        chunk: "",
        streamId: options.streamId,
        done: true,
      }),
    ]).then(() => undefined);
  });

  return runner;
}

function wireToolRunner(
  context: ReturnType<typeof resolveAdapterContext>,
  runner: OpenAIEventStreamLike,
  options: OpenAIAdapterOptions
): OpenAIEventStreamLike {
  const pendingToolCalls: PendingToolCall[] = [];
  let iteration = 0;

  runner.on("content", (delta) => {
    if (typeof delta !== "string") {
      return;
    }

    return reportStreamChunk(context, {
      chunk: delta,
      streamId: options.streamId,
    });
  });

  runner.on("functionToolCall", (functionCall) => {
    if (
      functionCall == null ||
      typeof functionCall !== "object" ||
      typeof (functionCall as OpenAIToolCall).name !== "string" ||
      typeof (functionCall as OpenAIToolCall).arguments !== "string"
    ) {
      return;
    }

    pendingToolCalls.push({
      ...(functionCall as OpenAIToolCall),
      startedAt: Date.now(),
    });
  });

  runner.on("functionToolCallResult", (result) => {
    const toolCall = pendingToolCalls.shift();
    if (toolCall == null || typeof result !== "string") {
      return;
    }

    iteration += 1;
    context.recordIteration?.();

    return Promise.all([
      reportToolEvent(context, {
        toolName: toolCall.name,
        input: parseJsonish(toolCall.arguments),
        output: parseJsonish(result),
        durationMs: Date.now() - toolCall.startedAt,
        status: "completed",
      }),
      reportCheckpointState(
        context,
        {
          source: options.checkpoint?.source ?? "openai.runTools",
          onStepFinish: async () => ({
            iteration,
            toolName: toolCall.name,
            phase: "tool_result",
          }),
        },
        result
      ),
    ]).then(() => undefined);
  });

  runner.on("finalChatCompletion", (completion) => {
    if (
      completion == null ||
      typeof completion !== "object" ||
      typeof (completion as OpenAIChatCompletion).model !== "string"
    ) {
      return;
    }

    return Promise.all([
      reportUsage(context, completion as OpenAIChatCompletion),
      reportStreamChunk(context, {
        chunk: "",
        streamId: options.streamId,
        done: true,
      }),
    ]).then(() => undefined);
  });

  return runner;
}

function wrapResponses<TResponses extends OpenAIResponsesLike>(
  responses: TResponses,
  context: ReturnType<typeof resolveAdapterContext>
): TResponses {
  const create = responses.create.bind(responses);

  return {
    ...responses,
    create(...args: unknown[]) {
      budgetGuard(context);
      const responsePromise = create(...args);
      Promise.resolve(responsePromise)
        .then((response) => {
          if (response.usage == null || response.model == null) {
            return;
          }

          return reportUsageEvent(context, {
            provider: "openai",
            model: response.model,
            promptTokens: response.usage.prompt_tokens,
            completionTokens: response.usage.completion_tokens,
            totalTokens: response.usage.total_tokens,
            ...(response.usage.prompt_tokens_details?.cached_tokens == null
              ? {}
              : {
                  promptTokenDetails: {
                    cacheReadTokens:
                      response.usage.prompt_tokens_details.cached_tokens,
                  },
                }),
          });
        })
        .catch((err: unknown) => {
          if (typeof console !== "undefined") {
            console.warn("[strait] telemetry delivery failed:", err);
          }
        });
      return responsePromise;
    },
  } satisfies OpenAIResponsesLike as TResponses;
}

export function withStrait<TClient extends OpenAIClientLike>(
  client: TClient,
  options: OpenAIAdapterOptions = {}
): TClient {
  const context = resolveAdapterContext(options);
  const create = client.chat.completions.create.bind(client.chat.completions);
  const stream = client.chat.completions.stream.bind(client.chat.completions);
  const runTools = client.chat.completions.runTools.bind(
    client.chat.completions
  );

  const wrappedCompletions = {
    ...client.chat.completions,
    create(...args: unknown[]) {
      budgetGuard(context);

      const completionPromise = create(...args);
      Promise.resolve(completionPromise)
        .then((completion) => reportUsage(context, completion))
        .catch((err: unknown) => {
          if (typeof console !== "undefined") {
            console.warn("[strait] telemetry delivery failed:", err);
          }
        });
      return completionPromise;
    },
    stream(...args: unknown[]) {
      budgetGuard(context);
      const runner = stream(...args);
      return wireStream(context, runner, options);
    },
    runTools(...args: unknown[]) {
      budgetGuard(context);
      const runner = runTools(...args);
      return wireToolRunner(context, runner, options);
    },
  } satisfies OpenAICompletionsLike;

  const wrappedChat = {
    ...client.chat,
    completions: wrappedCompletions,
  };

  const wrappedResponses =
    client.responses == null
      ? undefined
      : wrapResponses(client.responses, context);

  return new Proxy(client, {
    get(target, prop, receiver) {
      if (prop === "chat") {
        return wrappedChat;
      }

      if (prop === "responses") {
        return wrappedResponses;
      }

      return Reflect.get(target, prop, receiver);
    },
  });
}

export function createOpenAIAdapter<TClient extends OpenAIClientLike>(
  client: TClient,
  context: AdapterContextInput,
  options: Omit<OpenAIAdapterOptions, "context"> = {}
): TClient {
  return withStrait(client, {
    ...options,
    context: createAdapterContext(context),
  });
}
