import type { AdapterContextInput, AdapterTelemetryOptions } from "./internal";
import {
  budgetGuard,
  createAdapterContext,
  reportCheckpointState,
  reportStreamChunk,
  reportToolEvent,
  reportUsageEvent,
  resolveAdapterContext,
  toJsonValue,
} from "./internal";

type AnthropicUsage = {
  input_tokens: number;
  output_tokens: number;
};

type AnthropicToolUseBlock = {
  id?: string;
  input: unknown;
  name: string;
  type: "tool_use";
};

type AnthropicContentBlock =
  | AnthropicToolUseBlock
  | {
      text: string;
      type: "text";
    }
  | Record<string, unknown>;

type AnthropicMessage = {
  content: AnthropicContentBlock[];
  model: string;
  stop_reason?: string | null;
  usage?: AnthropicUsage;
};

type AnthropicMessageStreamLike = {
  on(event: string, listener: (...args: unknown[]) => unknown): unknown;
};

type AnthropicMessagesLike = {
  create: (...args: unknown[]) => PromiseLike<AnthropicMessage>;
  stream: (...args: unknown[]) => AnthropicMessageStreamLike;
};

type AnthropicToolRunnerLike = {
  on(event: string, listener: (...args: unknown[]) => unknown): unknown;
  finalMessage?: () => PromiseLike<AnthropicMessage>;
};

type AnthropicBetaMessagesLike = {
  toolRunner: (...args: unknown[]) => AnthropicToolRunnerLike;
};

type AnthropicClientLike = {
  beta?: {
    messages?: AnthropicBetaMessagesLike;
  };
  messages: AnthropicMessagesLike;
} & Record<PropertyKey, unknown>;

export interface AnthropicAdapterOptions extends AdapterTelemetryOptions {}

async function reportUsage(
  context: ReturnType<typeof resolveAdapterContext>,
  message: AnthropicMessage
): Promise<void> {
  if (message.usage == null) {
    return;
  }

  await reportUsageEvent(context, {
    provider: "anthropic",
    model: message.model,
    promptTokens: message.usage.input_tokens,
    completionTokens: message.usage.output_tokens,
    totalTokens: message.usage.input_tokens + message.usage.output_tokens,
  });
}

async function reportToolUses(
  context: ReturnType<typeof resolveAdapterContext>,
  message: AnthropicMessage
): Promise<void> {
  const toolUses = message.content.filter(
    (block): block is AnthropicToolUseBlock =>
      block.type === "tool_use" &&
      typeof (block as AnthropicToolUseBlock).name === "string"
  );

  await Promise.all(
    toolUses.map((toolUse) =>
      reportToolEvent(context, {
        toolName: toolUse.name,
        input: toJsonValue(toolUse.input),
        status: "requested",
      })
    )
  );
}

function wireMessageStream(
  context: ReturnType<typeof resolveAdapterContext>,
  runner: AnthropicMessageStreamLike,
  options: AnthropicAdapterOptions
): AnthropicMessageStreamLike {
  runner.on("text", (delta) => {
    if (typeof delta !== "string") {
      return;
    }

    return reportStreamChunk(context, {
      chunk: delta,
      streamId: options.streamId,
    });
  });

  runner.on("finalMessage", (message) => {
    if (
      message == null ||
      typeof message !== "object" ||
      typeof (message as AnthropicMessage).model !== "string" ||
      !Array.isArray((message as AnthropicMessage).content)
    ) {
      return;
    }

    return Promise.all([
      reportUsage(context, message as AnthropicMessage),
      reportToolUses(context, message as AnthropicMessage),
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
  runner: AnthropicToolRunnerLike,
  options: AnthropicAdapterOptions
): AnthropicToolRunnerLike {
  let iteration = 0;

  runner.on("text", (delta) => {
    if (typeof delta !== "string") {
      return;
    }

    return reportStreamChunk(context, {
      chunk: delta,
      streamId: options.streamId,
    });
  });

  runner.on("finalMessage", (message) => {
    if (
      message == null ||
      typeof message !== "object" ||
      typeof (message as AnthropicMessage).model !== "string" ||
      !Array.isArray((message as AnthropicMessage).content)
    ) {
      return;
    }

    iteration += 1;
    context.recordIteration?.();

    return Promise.all([
      reportUsage(context, message as AnthropicMessage),
      reportToolUses(context, message as AnthropicMessage),
      reportCheckpointState(
        context,
        {
          source: options.checkpoint?.source ?? "anthropic.toolRunner",
          onStepFinish: async () => ({
            iteration,
            phase: "tool_runner_iteration",
            stopReason: (message as AnthropicMessage).stop_reason ?? null,
          }),
        },
        message
      ),
      reportStreamChunk(context, {
        chunk: "",
        streamId: options.streamId,
        done: true,
      }),
    ]).then(() => undefined);
  });

  return runner;
}

export function withStrait<TClient extends AnthropicClientLike>(
  client: TClient,
  options: AnthropicAdapterOptions = {}
): TClient {
  const context = resolveAdapterContext(options);
  const create = client.messages.create.bind(client.messages);
  const stream = client.messages.stream.bind(client.messages);
  const toolRunner = client.beta?.messages?.toolRunner?.bind(
    client.beta.messages
  );

  const wrappedMessages = {
    ...client.messages,
    create(...args: unknown[]) {
      budgetGuard(context);
      const messagePromise = create(...args);
      Promise.resolve(messagePromise)
        .then(async (message) => {
          await reportUsage(context, message);
          await reportToolUses(context, message);
        })
        .catch((err: unknown) => {
          if (typeof console !== "undefined") {
            console.warn("[strait] telemetry delivery failed:", err);
          }
        });
      return messagePromise;
    },
    stream(...args: unknown[]) {
      budgetGuard(context);
      const runner = stream(...args);
      return wireMessageStream(context, runner, options);
    },
  } satisfies AnthropicMessagesLike;

  const wrappedBetaMessages =
    toolRunner == null
      ? client.beta?.messages
      : ({
          ...client.beta?.messages,
          toolRunner(...args: unknown[]) {
            budgetGuard(context);
            const runner = toolRunner(...args);
            return wireToolRunner(context, runner, options);
          },
        } satisfies AnthropicBetaMessagesLike);

  return new Proxy(client, {
    get(target, prop, receiver) {
      if (prop === "messages") {
        return wrappedMessages;
      }

      if (prop === "beta") {
        return {
          ...client.beta,
          messages: wrappedBetaMessages,
        };
      }

      return Reflect.get(target, prop, receiver);
    },
  });
}

export function createAnthropicAdapter<TClient extends AnthropicClientLike>(
  client: TClient,
  context: AdapterContextInput,
  options: Omit<AnthropicAdapterOptions, "context"> = {}
): TClient {
  return withStrait(client, {
    ...options,
    context: createAdapterContext(context),
  });
}
