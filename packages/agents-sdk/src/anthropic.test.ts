import { describe, expect, it, vi } from "vitest";

import { createAnthropicAdapter } from "./anthropic";

class FakeMessageStream {
  readonly listeners = new Map<
    string,
    Array<(...args: unknown[]) => unknown>
  >();

  on(event: string, listener: (...args: unknown[]) => unknown): this {
    const existing = this.listeners.get(event) ?? [];
    existing.push(listener);
    this.listeners.set(event, existing);
    return this;
  }

  emit(event: string, ...args: unknown[]): void {
    for (const listener of this.listeners.get(event) ?? []) {
      listener(...args);
    }
  }
}

type FakeMessage = {
  content: Array<
    | {
        text: string;
        type: "text";
      }
    | {
        input: unknown;
        name: string;
        type: "tool_use";
      }
  >;
  model: string;
  usage?: {
    input_tokens: number;
    output_tokens: number;
  };
};

type FakeMessages = {
  create: (...args: unknown[]) => Promise<FakeMessage>;
  stream: (...args: unknown[]) => FakeMessageStream;
};

type FakeClient = {
  foo?: string;
  messages: FakeMessages;
};

function flushTasks(): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, 10);
  });
}

describe("createAnthropicAdapter", () => {
  it("wraps create and reports usage while preserving the client surface", async () => {
    const create = vi.fn<FakeMessages["create"]>().mockResolvedValue({
      content: [{ type: "text", text: "hello" }],
      model: "claude-sonnet-4-5",
      usage: {
        input_tokens: 9,
        output_tokens: 4,
      },
    });

    const stream = vi.fn<FakeMessages["stream"]>();
    const client: FakeClient = {
      foo: "bar",
      messages: {
        create,
        stream,
      },
    };

    const reportUsage = vi.fn().mockResolvedValue(undefined);
    const adapter = createAnthropicAdapter(
      client,
      {
        reportUsage,
        reportToolCall: vi.fn(),
        stream: vi.fn(),
      },
      {
        streamId: "assistant",
      }
    );

    const result = await adapter.messages.create({
      model: "claude-sonnet-4-5",
      max_tokens: 128,
      messages: [],
    } as never);

    expect(result).toEqual({
      content: [{ type: "text", text: "hello" }],
      model: "claude-sonnet-4-5",
      usage: {
        input_tokens: 9,
        output_tokens: 4,
      },
    });
    expect(reportUsage).toHaveBeenCalledWith({
      provider: "anthropic",
      model: "claude-sonnet-4-5",
      promptTokens: 9,
      completionTokens: 4,
      totalTokens: 13,
    });
    expect(adapter.foo).toBe("bar");
  });

  it("wraps create and records requested tool use blocks", async () => {
    const create = vi.fn<FakeMessages["create"]>().mockResolvedValue({
      content: [
        {
          type: "tool_use",
          name: "search",
          input: {
            query: "weather in madrid",
          },
        },
      ],
      model: "claude-sonnet-4-5",
      usage: {
        input_tokens: 14,
        output_tokens: 6,
      },
    });

    const reportToolCall = vi.fn().mockResolvedValue(undefined);
    const adapter = createAnthropicAdapter(
      {
        messages: {
          create,
          stream: vi.fn<FakeMessages["stream"]>(),
        },
      },
      {
        reportUsage: vi.fn().mockResolvedValue(undefined),
        reportToolCall,
        stream: vi.fn(),
      }
    );

    await adapter.messages.create({
      model: "claude-sonnet-4-5",
      max_tokens: 128,
      messages: [],
      tools: [],
    } as never);
    await flushTasks();

    expect(reportToolCall).toHaveBeenCalledWith({
      toolName: "search",
      input: {
        query: "weather in madrid",
      },
      status: "requested",
    });
  });

  it("wraps stream and forwards text deltas plus final usage", async () => {
    const runner = new FakeMessageStream();
    const stream = vi.fn<FakeMessages["stream"]>().mockReturnValue(runner);
    const client: FakeClient = {
      messages: {
        create: vi.fn<FakeMessages["create"]>(),
        stream,
      },
    };

    const reportUsage = vi.fn().mockResolvedValue(undefined);
    const reportToolCall = vi.fn().mockResolvedValue(undefined);
    const reportStream = vi.fn().mockResolvedValue(undefined);
    const adapter = createAnthropicAdapter(
      client,
      {
        reportUsage,
        reportToolCall,
        stream: reportStream,
      },
      {
        streamId: "claude-stream",
      }
    );

    const result = adapter.messages.stream({
      model: "claude-sonnet-4-5",
      max_tokens: 128,
      messages: [],
    } as never);

    runner.emit("text", "hello ");
    runner.emit("text", "world");
    runner.emit("finalMessage", {
      content: [
        {
          type: "tool_use",
          name: "search",
          input: {
            query: "weather",
          },
        },
      ],
      model: "claude-sonnet-4-5",
      usage: {
        input_tokens: 12,
        output_tokens: 8,
      },
    });

    await flushTasks();

    expect(result).toBe(runner);
    expect(reportStream).toHaveBeenNthCalledWith(1, {
      chunk: "hello ",
      streamId: "claude-stream",
    });
    expect(reportStream).toHaveBeenNthCalledWith(2, {
      chunk: "world",
      streamId: "claude-stream",
    });
    expect(reportStream).toHaveBeenNthCalledWith(3, {
      chunk: "",
      streamId: "claude-stream",
      done: true,
    });
    expect(reportUsage).toHaveBeenCalledWith({
      provider: "anthropic",
      model: "claude-sonnet-4-5",
      promptTokens: 12,
      completionTokens: 8,
      totalTokens: 20,
    });
    expect(reportToolCall).toHaveBeenCalledWith({
      toolName: "search",
      input: {
        query: "weather",
      },
      status: "requested",
    });
  });
});
