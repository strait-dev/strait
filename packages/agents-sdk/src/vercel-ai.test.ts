import { afterEach, describe, expect, it, vi } from "vitest";

const generateTextMock = vi.fn();
const streamTextMock = vi.fn();

vi.mock("ai", () => ({
  generateText: (...args: unknown[]) => generateTextMock(...args),
  streamText: (...args: unknown[]) => streamTextMock(...args),
}));

import { createVercelAIAdapter } from "./vercel-ai";

function createStepEvent() {
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
      inputTokens: 12,
      inputTokenDetails: {
        noCacheTokens: 12,
        cacheReadTokens: 0,
        cacheWriteTokens: 0,
      },
      outputTokens: 8,
      outputTokenDetails: {
        textTokens: 8,
        reasoningTokens: 0,
      },
      totalTokens: 20,
    },
    warnings: undefined,
    request: {},
    response: {
      messages: [],
    },
    providerMetadata: undefined,
  };
}

afterEach(() => {
  generateTextMock.mockReset();
  streamTextMock.mockReset();
});

describe("createVercelAIAdapter", () => {
  it("wraps generateText and records usage plus tool calls", async () => {
    const reportUsage = vi.fn().mockResolvedValue(undefined);
    const reportToolCall = vi.fn().mockResolvedValue(undefined);
    const checkpoint = vi.fn().mockResolvedValue(undefined);
    const stream = vi.fn().mockResolvedValue(undefined);
    const onStepFinish = vi.fn();
    const onToolCallFinish = vi.fn();
    const stepEvent = createStepEvent();

    generateTextMock.mockImplementationOnce(async (options: Record<string, unknown>) => {
      await (options.experimental_onToolCallFinish as (event: unknown) => Promise<void>)?.({
        stepNumber: 0,
        model: {
          provider: "openai",
          modelId: "gpt-4.1",
        },
        toolCall: {
          toolName: "search",
          input: {
            query: "weather",
          },
        },
        messages: [],
        abortSignal: undefined,
        durationMs: 24.4,
        functionId: undefined,
        metadata: undefined,
        experimental_context: undefined,
        success: true,
        output: {
          city: "Madrid",
        },
      });
      await (options.onStepFinish as (event: unknown) => Promise<void>)?.(stepEvent);
      await (options.onFinish as (event: unknown) => Promise<void>)?.({
        ...stepEvent,
        steps: [stepEvent],
        totalUsage: stepEvent.usage,
      });
      return {
        text: "done",
      };
    });

    const adapter = createVercelAIAdapter(
      {
        reportUsage,
        reportToolCall,
        checkpoint,
        stream,
      },
      {
        checkpoint: {
          source: "vercel-ai",
          onStepFinish: () => ({
            phase: "tool-loop",
          }),
        },
      }
    );

    const result = await adapter.generateText({
      model: {} as never,
      prompt: "hello",
      onStepFinish,
      experimental_onToolCallFinish: onToolCallFinish,
    });

    expect(result).toEqual({
      text: "done",
    });
    expect(reportUsage).toHaveBeenCalledWith({
      provider: "openai",
      model: "gpt-4.1",
      promptTokens: 12,
      completionTokens: 8,
      totalTokens: 20,
    });
    expect(reportToolCall).toHaveBeenCalledWith({
      toolName: "search",
      input: {
        query: "weather",
      },
      output: {
        city: "Madrid",
      },
      durationMs: 24,
      status: "completed",
    });
    expect(checkpoint).toHaveBeenCalledWith(
      {
        phase: "tool-loop",
      },
      {
        source: "vercel-ai",
      }
    );
    expect(onStepFinish).toHaveBeenCalledWith(stepEvent);
    expect(onToolCallFinish).toHaveBeenCalledTimes(1);
    expect(stream).not.toHaveBeenCalled();
  });

  it("wraps streamText and forwards streamed deltas", async () => {
    const reportUsage = vi.fn().mockResolvedValue(undefined);
    const reportToolCall = vi.fn().mockResolvedValue(undefined);
    const checkpoint = vi.fn().mockResolvedValue(undefined);
    const stream = vi.fn().mockResolvedValue(undefined);
    const onChunk = vi.fn();
    const onFinish = vi.fn();
    const stepEvent = createStepEvent();

    streamTextMock.mockImplementationOnce((options: Record<string, unknown>) => {
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
        await (options.onStepFinish as (event: unknown) => Promise<void>)?.(stepEvent);
        await (options.onFinish as (event: unknown) => Promise<void>)?.({
          ...stepEvent,
          steps: [stepEvent],
          totalUsage: stepEvent.usage,
        });
      });

      return {
        textStream: [],
      };
    });

    const adapter = createVercelAIAdapter(
      {
        reportUsage,
        reportToolCall,
        checkpoint,
        stream,
      },
      {
        streamId: "assistant",
      }
    );

    const result = adapter.streamText({
      model: {} as never,
      prompt: "hello",
      onChunk,
      onFinish,
    });

    await new Promise((resolve) => {
      setTimeout(resolve, 10);
    });

    expect(result).toEqual({
      textStream: [],
    });
    expect(stream).toHaveBeenNthCalledWith(1, {
      chunk: "hel",
      streamId: "assistant",
    });
    expect(stream).toHaveBeenNthCalledWith(2, {
      chunk: "lo",
      streamId: "assistant",
    });
    expect(stream).toHaveBeenNthCalledWith(3, {
      chunk: "",
      streamId: "assistant",
      done: true,
    });
    expect(reportUsage).toHaveBeenCalledTimes(1);
    expect(onChunk).toHaveBeenCalledTimes(2);
    expect(onFinish).toHaveBeenCalledTimes(1);
    expect(reportToolCall).not.toHaveBeenCalled();
  });
});
