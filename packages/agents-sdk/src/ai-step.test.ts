import { describe, expect, it, vi } from "vitest";

import { createAIStep } from "./ai-step";

class FakeOpenAIRunner {
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

  finalContent = vi.fn().mockResolvedValue("done");
}

class FakeAnthropicRunner {
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

  finalMessage = vi.fn().mockResolvedValue({
    content: [{ type: "text", text: "done" }],
  });
}

function createContext() {
  return {
    checkpoint: vi.fn().mockResolvedValue({ id: "ckpt-1" }),
    reportToolCall: vi.fn().mockResolvedValue({ id: "tool-1" }),
    reportUsage: vi.fn().mockResolvedValue({ id: "usage-1" }),
    stream: vi.fn().mockResolvedValue({ status: "ok" }),
    budgetExceeded: vi.fn(),
    budgetSnapshot: vi.fn().mockReturnValue({
      promptTokens: 0,
      completionTokens: 0,
      totalTokens: 0,
      costMicrousd: 0,
      toolCalls: 0,
      limits: {},
    }),
  };
}

describe("createAIStep", () => {
  it("wraps arbitrary async work with checkpoints", async () => {
    const context = createContext();
    const ai = createAIStep(context as never);

    const result = await ai.wrap("summarize", Promise.resolve("done"), {
      metadata: { provider: "openai" },
    });

    expect(result).toBe("done");
    expect(context.checkpoint).toHaveBeenNthCalledWith(
      1,
      {
        step: "summarize",
        phase: "started",
        metadata: {
          provider: "openai",
        },
      },
      {
        source: "ai-step",
      }
    );
    expect(context.checkpoint).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        step: "summarize",
        phase: "completed",
      }),
      {
        source: "ai-step",
      }
    );
  });

  it("runs provider-agnostic infer flows through wrapped OpenAI clients", async () => {
    const context = createContext();
    const ai = createAIStep(context as never);

    const client = {
      chat: {
        completions: {
          create: vi.fn().mockResolvedValue({
            model: "gpt-4.1",
            usage: {
              prompt_tokens: 12,
              completion_tokens: 8,
              total_tokens: 20,
            },
            output_text: "hello",
          }),
          stream: vi.fn(),
          runTools: vi.fn(),
        },
      },
    };

    const result = await ai.infer("classify", {
      provider: "openai",
      client,
      model: "gpt-4.1",
      request: {
        messages: [],
      },
      selectResult: (response) =>
        (response as { output_text: string }).output_text,
    });

    expect(result).toBe("hello");
    expect(client.chat.completions.create).toHaveBeenCalledWith({
      model: "gpt-4.1",
      messages: [],
    });
    expect(context.reportUsage).toHaveBeenCalledWith(
      expect.objectContaining({
        provider: "openai",
        model: "gpt-4.1",
        promptTokens: 12,
        completionTokens: 8,
        totalTokens: 20,
      })
    );
  });

  it("runs durable agent loops through wrapped runners", async () => {
    const context = createContext();
    const ai = createAIStep(context as never);
    const openAIRunner = new FakeOpenAIRunner();
    const anthropicRunner = new FakeAnthropicRunner();

    const openAIClient = {
      chat: {
        completions: {
          create: vi.fn(),
          stream: vi.fn(),
          runTools: vi.fn().mockReturnValue(openAIRunner),
        },
      },
    };

    const anthropicClient = {
      messages: {
        create: vi.fn(),
        stream: vi.fn(),
      },
      beta: {
        messages: {
          toolRunner: vi.fn().mockReturnValue(anthropicRunner),
        },
      },
    };

    const openAIResult = await ai.agentLoop("research-openai", {
      provider: "openai",
      client: openAIClient,
      request: {
        model: "gpt-4.1",
        messages: [],
      },
    });
    const anthropicResult = await ai.agentLoop("research-anthropic", {
      provider: "anthropic",
      client: anthropicClient,
      request: {
        model: "claude-sonnet-4-5",
        messages: [],
      },
    });

    expect(openAIResult).toBe("done");
    expect(anthropicResult).toEqual({
      content: [{ type: "text", text: "done" }],
    });
    expect(openAIRunner.finalContent).toHaveBeenCalledTimes(1);
    expect(anthropicRunner.finalMessage).toHaveBeenCalledTimes(1);
  });
});
