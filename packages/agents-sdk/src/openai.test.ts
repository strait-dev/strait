import { describe, expect, it, vi } from "vitest";

import { createOpenAIAdapter } from "./openai";

class FakeEventStream {
  readonly listeners = new Map<string, Array<(...args: unknown[]) => unknown>>();

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

type FakeCompletion = {
  model: string;
  usage?: {
    prompt_tokens: number;
    completion_tokens: number;
    total_tokens: number;
  };
};

type FakeCompletions = {
  create: (...args: unknown[]) => Promise<FakeCompletion>;
  stream: (...args: unknown[]) => FakeEventStream;
  runTools: (...args: unknown[]) => FakeEventStream;
};

type FakeClient = {
  foo?: string;
  chat: {
    completions: FakeCompletions;
  };
};

function flushTasks(): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, 10);
  });
}

describe("createOpenAIAdapter", () => {
  it("wraps create and reports usage while preserving the client surface", async () => {
    const create = vi.fn<FakeCompletions["create"]>().mockResolvedValue({
      model: "gpt-4.1",
      usage: {
        prompt_tokens: 11,
        completion_tokens: 7,
        total_tokens: 18,
      },
    });

    const stream = vi.fn<FakeCompletions["stream"]>();
    const runTools = vi.fn<FakeCompletions["runTools"]>();
    const client: FakeClient = {
      foo: "bar",
      chat: {
        completions: {
          create,
          stream,
          runTools,
        },
      },
    };

    const reportUsage = vi.fn().mockResolvedValue(undefined);
    const adapter = createOpenAIAdapter(
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

    const result = await adapter.chat.completions.create({
      model: "gpt-4.1",
      messages: [],
    } as never);

    expect(result).toEqual({
      model: "gpt-4.1",
      usage: {
        prompt_tokens: 11,
        completion_tokens: 7,
        total_tokens: 18,
      },
    });
    expect(reportUsage).toHaveBeenCalledWith({
      provider: "openai",
      model: "gpt-4.1",
      promptTokens: 11,
      completionTokens: 7,
      totalTokens: 18,
    });
    expect(adapter.foo).toBe("bar");
  });

  it("wraps stream and forwards content deltas plus terminal usage", async () => {
    const runner = new FakeEventStream();
    const stream = vi.fn<FakeCompletions["stream"]>().mockReturnValue(runner);
    const client: FakeClient = {
      chat: {
        completions: {
          create: vi.fn<FakeCompletions["create"]>(),
          stream,
          runTools: vi.fn<FakeCompletions["runTools"]>(),
        },
      },
    };

    const reportUsage = vi.fn().mockResolvedValue(undefined);
    const reportStream = vi.fn().mockResolvedValue(undefined);
    const adapter = createOpenAIAdapter(client, {
      reportUsage,
      reportToolCall: vi.fn(),
      stream: reportStream,
    });

    const result = adapter.chat.completions.stream({
      model: "gpt-4.1",
      messages: [],
    } as never);

    runner.emit("content", "hello ");
    runner.emit("content", "world");
    runner.emit("finalChatCompletion", {
      model: "gpt-4.1",
      usage: {
        prompt_tokens: 15,
        completion_tokens: 5,
        total_tokens: 20,
      },
    });

    await flushTasks();

    expect(result).toBe(runner);
    expect(reportStream).toHaveBeenNthCalledWith(1, {
      chunk: "hello ",
      streamId: undefined,
    });
    expect(reportStream).toHaveBeenNthCalledWith(2, {
      chunk: "world",
      streamId: undefined,
    });
    expect(reportStream).toHaveBeenNthCalledWith(3, {
      chunk: "",
      streamId: undefined,
      done: true,
    });
    expect(reportUsage).toHaveBeenCalledWith({
      provider: "openai",
      model: "gpt-4.1",
      promptTokens: 15,
      completionTokens: 5,
      totalTokens: 20,
    });
  });

  it("wraps runTools and records tool execution plus usage", async () => {
    const runner = new FakeEventStream();
    const runTools = vi.fn<FakeCompletions["runTools"]>().mockReturnValue(runner);
    const client: FakeClient = {
      chat: {
        completions: {
          create: vi.fn<FakeCompletions["create"]>(),
          stream: vi.fn<FakeCompletions["stream"]>(),
          runTools,
        },
      },
    };

    const reportUsage = vi.fn().mockResolvedValue(undefined);
    const reportToolCall = vi.fn().mockResolvedValue(undefined);
    const reportStream = vi.fn().mockResolvedValue(undefined);
    const adapter = createOpenAIAdapter(
      client,
      {
        reportUsage,
        reportToolCall,
        stream: reportStream,
      },
      {
        streamId: "tool-runner",
      }
    );

    const result = adapter.chat.completions.runTools({
      model: "gpt-4.1",
      messages: [],
      tools: [],
    } as never);

    runner.emit("content", "thinking");
    runner.emit("functionToolCall", {
      name: "search",
      arguments: "{\"query\":\"weather\"}",
    });
    runner.emit("functionToolCallResult", "{\"city\":\"Madrid\"}");
    runner.emit("finalChatCompletion", {
      model: "gpt-4.1",
      usage: {
        prompt_tokens: 21,
        completion_tokens: 13,
        total_tokens: 34,
      },
    });

    await flushTasks();

    expect(result).toBe(runner);
    expect(reportToolCall).toHaveBeenCalledWith({
      toolName: "search",
      input: {
        query: "weather",
      },
      output: {
        city: "Madrid",
      },
      status: "completed",
    });
    expect(reportUsage).toHaveBeenCalledWith({
      provider: "openai",
      model: "gpt-4.1",
      promptTokens: 21,
      completionTokens: 13,
      totalTokens: 34,
    });
    expect(reportStream).toHaveBeenNthCalledWith(1, {
      chunk: "thinking",
      streamId: "tool-runner",
    });
    expect(reportStream).toHaveBeenNthCalledWith(2, {
      chunk: "",
      streamId: "tool-runner",
      done: true,
    });
  });
});
