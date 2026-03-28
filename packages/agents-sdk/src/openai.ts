import type { StraitContext } from "./context";
import type { JsonValue } from "./types";

type AdapterContext = Pick<
  StraitContext,
  "reportToolCall" | "reportUsage" | "stream"
>;

type OpenAIUsage = {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
};

type OpenAIChatCompletion = {
  model: string;
  usage?: OpenAIUsage;
};

type OpenAIToolCall = {
  name: string;
  arguments: string;
};

type OpenAIEventStreamLike = {
  on(event: string, listener: (...args: unknown[]) => unknown): unknown;
};

type OpenAICompletionsLike = {
  create: (...args: unknown[]) => PromiseLike<OpenAIChatCompletion>;
  stream: (...args: unknown[]) => OpenAIEventStreamLike;
  runTools: (...args: unknown[]) => OpenAIEventStreamLike;
};

type OpenAIClientLike = {
  chat: {
    completions: OpenAICompletionsLike;
  };
} & Record<PropertyKey, unknown>;

export interface OpenAIAdapterOptions {
  streamId?: string;
}

function parseJSONish(value: string): JsonValue {
  try {
    return JSON.parse(value) as JsonValue;
  } catch {
    return value;
  }
}

async function reportUsage(
  context: AdapterContext,
  completion: OpenAIChatCompletion
): Promise<void> {
  if (completion.usage == null) {
    return;
  }

  await context.reportUsage({
    provider: "openai",
    model: completion.model,
    promptTokens: completion.usage.prompt_tokens,
    completionTokens: completion.usage.completion_tokens,
    totalTokens: completion.usage.total_tokens,
  });
}

function wireStream(
  context: AdapterContext,
  runner: OpenAIEventStreamLike,
  options?: OpenAIAdapterOptions
): OpenAIEventStreamLike {
  runner.on("content", (delta) => {
    if (typeof delta !== "string") {
      return;
    }
    return context.stream({
      chunk: delta,
      streamId: options?.streamId,
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
      context.stream({
        chunk: "",
        streamId: options?.streamId,
        done: true,
      }),
    ]).then(() => undefined);
  });

  return runner;
}

function wireToolRunner(
  context: AdapterContext,
  runner: OpenAIEventStreamLike,
  options?: OpenAIAdapterOptions
): OpenAIEventStreamLike {
  const pendingToolCalls: OpenAIToolCall[] = [];

  runner.on("content", (delta) => {
    if (typeof delta !== "string") {
      return;
    }

    return context.stream({
      chunk: delta,
      streamId: options?.streamId,
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

    pendingToolCalls.push(functionCall as OpenAIToolCall);
  });

  runner.on("functionToolCallResult", (result) => {
    const toolCall = pendingToolCalls.shift();
    if (toolCall == null || typeof result !== "string") {
      return;
    }

    return context.reportToolCall({
      toolName: toolCall.name,
      input: parseJSONish(toolCall.arguments),
      output: parseJSONish(result),
      status: "completed",
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
      context.stream({
        chunk: "",
        streamId: options?.streamId,
        done: true,
      }),
    ]).then(() => undefined);
  });

  return runner;
}

export function createOpenAIAdapter<TClient extends OpenAIClientLike>(
  client: TClient,
  context: AdapterContext,
  options?: OpenAIAdapterOptions
): TClient {
  const create = client.chat.completions.create.bind(client.chat.completions);
  const stream = client.chat.completions.stream.bind(client.chat.completions);
  const runTools = client.chat.completions.runTools.bind(
    client.chat.completions
  );

  const wrappedCompletions = {
    ...client.chat.completions,
    create(...args: unknown[]) {
      const completionPromise = create(...args);
      Promise.resolve(completionPromise)
        .then((completion) => reportUsage(context, completion))
        .catch(() => undefined);
      return completionPromise;
    },
    stream(...args: unknown[]) {
      const runner = stream(...args);
      return wireStream(context, runner, options);
    },
    runTools(...args: unknown[]) {
      const runner = runTools(...args);
      return wireToolRunner(context, runner, options);
    },
  } satisfies OpenAICompletionsLike;

  const wrappedChat = {
    ...client.chat,
    completions: wrappedCompletions,
  };

  return new Proxy(client, {
    get(target, prop, receiver) {
      if (prop === "chat") {
        return wrappedChat;
      }
      return Reflect.get(target, prop, receiver);
    },
  });
}
