import { describe, expect, test } from "bun:test";
import { createStraitProvider } from "../src/ai/provider";
import { createTestContext } from "../src/authoring/test-client";

const mockUsage = (input: number, output: number) => ({
  inputTokens: {
    total: input,
    noCache: undefined,
    cacheRead: undefined,
    cacheWrite: undefined,
  },
  outputTokens: {
    total: output,
    noCache: undefined,
    cacheRead: undefined,
    cacheWrite: undefined,
  },
});

const emptyGenResult = {
  content: [],
  finishReason: "stop" as const,
  warnings: [],
  usage: mockUsage(0, 0),
};

describe("createStraitProvider", () => {
  test("wrapGenerate reports usage after doGenerate", async () => {
    const { ctx, record } = createTestContext("run_ai");
    const provider = createStraitProvider(ctx, { providerName: "openai" });

    const mockResult = {
      content: [],
      finishReason: "stop" as const,
      warnings: [],
      usage: mockUsage(100, 50),
    };

    const result = await provider.wrapGenerate?.({
      doGenerate: (async () => mockResult) as never,
      doStream: (async () => ({ stream: new ReadableStream() })) as never,
      params: {} as never,
      model: {} as never,
    });

    expect(result).toBe(mockResult as unknown as typeof result);
    expect(record.usageReports).toHaveLength(1);
    expect(record.usageReports[0].provider).toBe("openai");
    expect(record.usageReports[0].promptTokens).toBe(100);
    expect(record.usageReports[0].completionTokens).toBe(50);
    expect(record.usageReports[0].totalTokens).toBe(150);
  });

  test("wrapGenerate logs tool calls from content", async () => {
    const { ctx, record } = createTestContext("run_ai");
    const provider = createStraitProvider(ctx, { providerName: "anthropic" });

    await provider.wrapGenerate?.({
      doGenerate: (async () => ({
        content: [
          {
            type: "tool-call" as const,
            toolCallId: "tc_1",
            toolName: "search",
            input: JSON.stringify({ query: "test" }),
          },
          {
            type: "tool-call" as const,
            toolCallId: "tc_2",
            toolName: "calculate",
            input: JSON.stringify({ expression: "1+1" }),
          },
        ],
        finishReason: "tool-calls" as const,
        warnings: [],
        usage: mockUsage(50, 25),
      })) as never,
      doStream: (async () => ({ stream: new ReadableStream() })) as never,
      params: {} as never,
      model: {} as never,
    });

    expect(record.toolCalls).toHaveLength(2);
    expect(record.toolCalls[0].toolName).toBe("search");
    expect(record.toolCalls[0].input).toEqual({ query: "test" });
    expect(record.toolCalls[1].toolName).toBe("calculate");
  });

  test("wrapGenerate skips usage when reportUsage is false", async () => {
    const { ctx, record } = createTestContext("run_ai");
    const provider = createStraitProvider(ctx, { reportUsage: false });

    await provider.wrapGenerate?.({
      doGenerate: (async () => ({
        content: [
          {
            type: "tool-call" as const,
            toolCallId: "tc_1",
            toolName: "test",
            input: "{}",
          },
        ],
        finishReason: "tool-calls" as const,
        warnings: [],
        usage: mockUsage(100, 50),
      })) as never,
      doStream: (async () => ({ stream: new ReadableStream() })) as never,
      params: {} as never,
      model: {} as never,
    });

    expect(record.usageReports).toHaveLength(0);
    expect(record.toolCalls).toHaveLength(1);
  });

  test("wrapGenerate skips tool call logging when logToolCalls is false", async () => {
    const { ctx, record } = createTestContext("run_ai");
    const provider = createStraitProvider(ctx, { logToolCalls: false });

    await provider.wrapGenerate?.({
      doGenerate: (async () => ({
        content: [
          {
            type: "tool-call" as const,
            toolCallId: "tc_1",
            toolName: "test",
            input: "{}",
          },
        ],
        finishReason: "tool-calls" as const,
        warnings: [],
        usage: mockUsage(100, 50),
      })) as never,
      doStream: (async () => ({ stream: new ReadableStream() })) as never,
      params: {} as never,
      model: {} as never,
    });

    expect(record.usageReports).toHaveLength(1);
    expect(record.toolCalls).toHaveLength(0);
  });

  test("wrapStream forwards text chunks to ctx.streamChunk", async () => {
    const { ctx, record } = createTestContext("run_ai");
    const provider = createStraitProvider(ctx, { providerName: "openai" });

    const chunks = [
      { type: "text-delta" as const, id: "t_1", delta: "Hello" },
      { type: "text-delta" as const, id: "t_2", delta: " world" },
      {
        type: "finish" as const,
        usage: mockUsage(0, 0),
        finishReason: "stop" as const,
      },
    ];

    const mockStream = new ReadableStream({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(chunk);
        }
        controller.close();
      },
    });

    // biome-ignore lint/style/noNonNullAssertion: test assertion
    const result = await provider.wrapStream!({
      doStream: (async () => ({ stream: mockStream })) as never,
      doGenerate: (async () => emptyGenResult) as never,
      params: {} as never,
      model: {} as never,
    });

    // Consume the stream to trigger the transform
    const reader = result.stream.getReader();
    const readChunks: unknown[] = [];
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      readChunks.push(value);
    }

    // Wait for fire-and-forget promises to settle
    await new Promise((resolve) => setTimeout(resolve, 10));

    expect(readChunks).toHaveLength(3);
    expect(record.streamChunks.length).toBeGreaterThanOrEqual(2);
    expect(record.streamChunks[0].chunk).toBe("Hello");
    expect(record.streamChunks[1].chunk).toBe(" world");
  });

  test("wrapStream skips streaming when streamToStrait is false", async () => {
    const { ctx, record } = createTestContext("run_ai");
    const provider = createStraitProvider(ctx, { streamToStrait: false });

    const mockStream = new ReadableStream({
      start(controller) {
        controller.enqueue({
          type: "text-delta" as const,
          id: "t_1",
          delta: "Hello",
        });
        controller.close();
      },
    });

    // biome-ignore lint/style/noNonNullAssertion: test assertion
    const result = await provider.wrapStream!({
      doStream: (async () => ({ stream: mockStream })) as never,
      doGenerate: (async () => emptyGenResult) as never,
      params: {} as never,
      model: {} as never,
    });

    // Stream should be the original (not transformed)
    const reader = result.stream.getReader();
    while (true) {
      const { done } = await reader.read();
      if (done) {
        break;
      }
    }

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(record.streamChunks).toHaveLength(0);
  });
});
