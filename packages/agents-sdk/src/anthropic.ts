import type { StraitContext } from "./context";
import type { JsonValue } from "./types";

type AdapterContext = Pick<
  StraitContext,
  "reportToolCall" | "reportUsage" | "stream"
>;

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
      type: "text";
      text: string;
    }
  | Record<string, unknown>;

type AnthropicMessage = {
  content: AnthropicContentBlock[];
  model: string;
  usage?: AnthropicUsage;
};

type AnthropicMessageStreamLike = {
  on(event: string, listener: (...args: unknown[]) => unknown): unknown;
};

type AnthropicMessagesLike = {
  create: (...args: unknown[]) => PromiseLike<AnthropicMessage>;
  stream: (...args: unknown[]) => AnthropicMessageStreamLike;
};

type AnthropicClientLike = {
  messages: AnthropicMessagesLike;
} & Record<PropertyKey, unknown>;

export interface AnthropicAdapterOptions {
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

async function reportUsage(
  context: AdapterContext,
  message: AnthropicMessage
): Promise<void> {
  if (message.usage == null) {
    return;
  }

  await context.reportUsage({
    provider: "anthropic",
    model: message.model,
    promptTokens: message.usage.input_tokens,
    completionTokens: message.usage.output_tokens,
    totalTokens: message.usage.input_tokens + message.usage.output_tokens,
  });
}

async function reportToolUses(
  context: AdapterContext,
  message: AnthropicMessage
): Promise<void> {
  const toolUses = message.content.filter(
    (block): block is AnthropicToolUseBlock =>
      block.type === "tool_use" &&
      typeof (block as AnthropicToolUseBlock).name === "string"
  );

  await Promise.all(
    toolUses.map((toolUse) =>
      context.reportToolCall({
        toolName: toolUse.name,
        input: toJSONValue(toolUse.input),
        status: "requested",
      })
    )
  );
}

function wireStream(
  context: AdapterContext,
  runner: AnthropicMessageStreamLike,
  options?: AnthropicAdapterOptions
): AnthropicMessageStreamLike {
  runner.on("text", (delta) => {
    if (typeof delta !== "string") {
      return;
    }

    return context.stream({
      chunk: delta,
      streamId: options?.streamId,
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
      context.stream({
        chunk: "",
        streamId: options?.streamId,
        done: true,
      }),
    ]).then(() => undefined);
  });

  return runner;
}

export function createAnthropicAdapter<TClient extends AnthropicClientLike>(
  client: TClient,
  context: AdapterContext,
  options?: AnthropicAdapterOptions
): TClient {
  const create = client.messages.create.bind(client.messages);
  const stream = client.messages.stream.bind(client.messages);

  const wrappedMessages = {
    ...client.messages,
    create(...args: unknown[]) {
      const messagePromise = create(...args);
      Promise.resolve(messagePromise)
        .then(async (message) => {
          await reportUsage(context, message);
          await reportToolUses(context, message);
        })
        .catch(() => undefined);
      return messagePromise;
    },
    stream(...args: unknown[]) {
      const runner = stream(...args);
      return wireStream(context, runner, options);
    },
  } satisfies AnthropicMessagesLike;

  return new Proxy(client, {
    get(target, prop, receiver) {
      if (prop === "messages") {
        return wrappedMessages;
      }

      return Reflect.get(target, prop, receiver);
    },
  });
}
