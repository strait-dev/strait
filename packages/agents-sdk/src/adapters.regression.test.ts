import { describe, expect, it, vi } from "vitest";

const generateTextMock = vi.fn();
const streamTextMock = vi.fn();

vi.mock("ai", () => ({
  generateText: (...args: unknown[]) => generateTextMock(...args),
  streamText: (...args: unknown[]) => streamTextMock(...args),
}));

import { createAnthropicAdapter } from "./anthropic";
import { createOpenAIAdapter } from "./openai";
import { createVercelAIAdapter } from "./vercel-ai";

class FakeOpenAIStream {
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

class FakeAnthropicStream {
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

function createStepEvent(inputTokens: number, outputTokens: number) {
  return {
    stepNumber: 0,
    model: {
      provider: "openai",
      modelId: "gpt-4.1",
    },
    functionId: undefined,
    metadata: undefined,
    experimental_context: undefined,
    content: [],
    text: "done",
    reasoning: [],
    reasoningText: undefined,
    files: [],
    sources: [],
    toolCalls: [],
    staticToolCalls: [],
    dynamicToolCalls: [],
    toolResults: [],
    staticToolResults: [],
    dynamicToolResults: [],
    finishReason: "stop",
    rawFinishReason: "stop",
    usage: {
      inputTokens,
      inputTokenDetails: {
        noCacheTokens: inputTokens,
        cacheReadTokens: 0,
        cacheWriteTokens: 0,
      },
      outputTokens,
      outputTokenDetails: {
        textTokens: outputTokens,
        reasoningTokens: 0,
      },
      totalTokens: inputTokens + outputTokens,
    },
    warnings: undefined,
    request: {},
    response: {
      messages: [],
    },
    providerMetadata: undefined,
  };
}

function flushTasks(): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, 10);
  });
}

type UsageReporter = Parameters<typeof createOpenAIAdapter>[1]["reportUsage"];
type StreamReporter = Parameters<typeof createOpenAIAdapter>[1]["stream"];

type UsageSnapshot = {
  completionTokens: number;
  promptTokens: number;
  totalTokens: number | undefined;
};

type StreamSnapshot = {
  chunk: string;
  done: boolean | undefined;
  streamId: string | undefined;
};

function createUsageReporter(reports: UsageSnapshot[]): UsageReporter {
  return (report) => {
    reports.push({
      completionTokens: report.completionTokens,
      promptTokens: report.promptTokens,
      totalTokens: report.totalTokens,
    });

    return Promise.resolve({} as never);
  };
}

function createStreamReporter(chunks: StreamSnapshot[]): StreamReporter {
  return (chunk) => {
    chunks.push({
      chunk: chunk.chunk,
      done: chunk.done,
      streamId: chunk.streamId,
    });

    return Promise.resolve({
      status: "ok",
    });
  };
}

function createDeterministicRandom(seed: number): () => number {
  let state = seed;

  return () => {
    state = (state * 1_664_525 + 1_013_904_223) % 4_294_967_296;
    return state / 4_294_967_296;
  };
}

describe("adapter regression coverage", () => {
  it("keeps usage token accounting structurally aligned across adapters", async () => {
    const vercelReports: UsageSnapshot[] = [];
    const openAIReports: UsageSnapshot[] = [];
    const anthropicReports: UsageSnapshot[] = [];

    generateTextMock.mockImplementationOnce(
      async (options: Record<string, unknown>) => {
        const stepEvent = createStepEvent(12, 8);
        await (options.onStepFinish as (event: unknown) => Promise<void>)?.(
          stepEvent
        );
        return {
          text: "done",
        };
      }
    );

    const vercelAdapter = createVercelAIAdapter({
      checkpoint: vi.fn(),
      reportToolCall: vi.fn(),
      reportUsage: createUsageReporter(vercelReports),
      stream: vi.fn(),
    });

    const openAIAdapter = createOpenAIAdapter(
      {
        chat: {
          completions: {
            create: vi.fn().mockResolvedValue({
              model: "gpt-4.1",
              usage: {
                prompt_tokens: 12,
                completion_tokens: 8,
                total_tokens: 20,
              },
            }),
            stream: vi.fn(),
            runTools: vi.fn(),
          },
        },
      },
      {
        reportToolCall: vi.fn(),
        reportUsage: createUsageReporter(openAIReports),
        stream: vi.fn(),
      }
    );

    const anthropicAdapter = createAnthropicAdapter(
      {
        messages: {
          create: vi.fn().mockResolvedValue({
            content: [{ type: "text", text: "done" }],
            model: "claude-sonnet-4-5",
            usage: {
              input_tokens: 12,
              output_tokens: 8,
            },
          }),
          stream: vi.fn(),
        },
      },
      {
        reportToolCall: vi.fn(),
        reportUsage: createUsageReporter(anthropicReports),
        stream: vi.fn(),
      }
    );

    await vercelAdapter.generateText({
      model: {} as never,
      prompt: "hello",
    });
    await openAIAdapter.chat.completions.create({
      model: "gpt-4.1",
      messages: [],
    } as never);
    await anthropicAdapter.messages.create({
      model: "claude-sonnet-4-5",
      max_tokens: 128,
      messages: [],
    } as never);
    await flushTasks();

    expect(vercelReports).toHaveLength(1);
    expect(openAIReports).toHaveLength(1);
    expect(anthropicReports).toHaveLength(1);

    const vercelReport = vercelReports.at(0);
    const openAIReport = openAIReports.at(0);
    const anthropicReport = anthropicReports.at(0);

    if (
      vercelReport == null ||
      openAIReport == null ||
      anthropicReport == null
    ) {
      throw new Error("expected usage telemetry from every adapter");
    }

    const comparable = [vercelReport, openAIReport, anthropicReport];

    expect(comparable).toEqual([
      {
        promptTokens: 12,
        completionTokens: 8,
        totalTokens: 20,
      },
      {
        promptTokens: 12,
        completionTokens: 8,
        totalTokens: 20,
      },
      {
        promptTokens: 12,
        completionTokens: 8,
        totalTokens: 20,
      },
    ]);
  });

  it("keeps streaming chunk telemetry aligned across adapters", async () => {
    const vercelChunks: StreamSnapshot[] = [];
    const openAIChunks: StreamSnapshot[] = [];
    const anthropicChunks: StreamSnapshot[] = [];

    streamTextMock.mockImplementationOnce(
      (options: Record<string, unknown>) => {
        Promise.resolve().then(async () => {
          await (options.onChunk as (event: unknown) => Promise<void>)?.({
            chunk: {
              type: "text-delta",
              id: "stream-1",
              text: "hel",
            },
          });
          await (options.onChunk as (event: unknown) => Promise<void>)?.({
            chunk: {
              type: "text-delta",
              id: "stream-1",
              text: "lo",
            },
          });
          await (options.onFinish as (event: unknown) => Promise<void>)?.({});
        });

        return {
          textStream: [],
        };
      }
    );

    const openAIRunner = new FakeOpenAIStream();
    const anthropicRunner = new FakeAnthropicStream();

    const vercelAdapter = createVercelAIAdapter(
      {
        checkpoint: vi.fn(),
        reportToolCall: vi.fn(),
        reportUsage: vi.fn(),
        stream: createStreamReporter(vercelChunks),
      },
      {
        streamId: "assistant",
      }
    );

    const openAIAdapter = createOpenAIAdapter(
      {
        chat: {
          completions: {
            create: vi.fn(),
            stream: vi.fn().mockReturnValue(openAIRunner),
            runTools: vi.fn(),
          },
        },
      },
      {
        reportToolCall: vi.fn(),
        reportUsage: vi.fn(),
        stream: createStreamReporter(openAIChunks),
      },
      {
        streamId: "assistant",
      }
    );

    const anthropicAdapter = createAnthropicAdapter(
      {
        messages: {
          create: vi.fn(),
          stream: vi.fn().mockReturnValue(anthropicRunner),
        },
      },
      {
        reportToolCall: vi.fn(),
        reportUsage: vi.fn(),
        stream: createStreamReporter(anthropicChunks),
      },
      {
        streamId: "assistant",
      }
    );

    vercelAdapter.streamText({
      model: {} as never,
      prompt: "hello",
    });
    openAIAdapter.chat.completions.stream({
      model: "gpt-4.1",
      messages: [],
    } as never);
    anthropicAdapter.messages.stream({
      model: "claude-sonnet-4-5",
      max_tokens: 128,
      messages: [],
    } as never);

    openAIRunner.emit("content", "hel");
    openAIRunner.emit("content", "lo");
    openAIRunner.emit("finalChatCompletion", {
      model: "gpt-4.1",
      usage: {
        prompt_tokens: 12,
        completion_tokens: 8,
        total_tokens: 20,
      },
    });

    anthropicRunner.emit("text", "hel");
    anthropicRunner.emit("text", "lo");
    anthropicRunner.emit("finalMessage", {
      content: [{ type: "text", text: "hello" }],
      model: "claude-sonnet-4-5",
      usage: {
        input_tokens: 12,
        output_tokens: 8,
      },
    });

    await flushTasks();

    expect(vercelChunks).toEqual([
      {
        chunk: "hel",
        streamId: "assistant",
      },
      {
        chunk: "lo",
        streamId: "assistant",
      },
      {
        chunk: "",
        streamId: "assistant",
        done: true,
      },
    ]);
    expect(openAIChunks).toEqual(vercelChunks);
    expect(anthropicChunks).toEqual(vercelChunks);
  });

  it("keeps randomized usage totals consistent for provider wrappers", async () => {
    const random = createDeterministicRandom(1234);

    for (let index = 0; index < 25; index += 1) {
      const promptTokens = Math.floor(random() * 500);
      const completionTokens = Math.floor(random() * 500);
      const openAIReports: UsageSnapshot[] = [];
      const anthropicReports: UsageSnapshot[] = [];

      const openAIAdapter = createOpenAIAdapter(
        {
          chat: {
            completions: {
              create: vi.fn().mockResolvedValue({
                model: "gpt-4.1",
                usage: {
                  prompt_tokens: promptTokens,
                  completion_tokens: completionTokens,
                  total_tokens: promptTokens + completionTokens,
                },
              }),
              stream: vi.fn(),
              runTools: vi.fn(),
            },
          },
        },
        {
          reportToolCall: vi.fn(),
          reportUsage: createUsageReporter(openAIReports),
          stream: vi.fn(),
        }
      );

      const anthropicAdapter = createAnthropicAdapter(
        {
          messages: {
            create: vi.fn().mockResolvedValue({
              content: [{ type: "text", text: "done" }],
              model: "claude-sonnet-4-5",
              usage: {
                input_tokens: promptTokens,
                output_tokens: completionTokens,
              },
            }),
            stream: vi.fn(),
          },
        },
        {
          reportToolCall: vi.fn(),
          reportUsage: createUsageReporter(anthropicReports),
          stream: vi.fn(),
        }
      );

      await openAIAdapter.chat.completions.create({
        model: "gpt-4.1",
        messages: [],
      } as never);
      await anthropicAdapter.messages.create({
        model: "claude-sonnet-4-5",
        max_tokens: 128,
        messages: [],
      } as never);
      await flushTasks();

      expect(openAIReports.at(0)).toMatchObject({
        promptTokens,
        completionTokens,
        totalTokens: promptTokens + completionTokens,
      });
      expect(anthropicReports.at(0)).toMatchObject({
        promptTokens,
        completionTokens,
        totalTokens: promptTokens + completionTokens,
      });
    }
  });
});
