import { describe, expect, test } from "bun:test";
import { createStraitProvider } from "../src/ai/provider";
import { createTestContext } from "../src/authoring/test-client";

describe("createStraitProvider", () => {
  test("wrapGenerate reports usage after doGenerate", async () => {
    const { ctx, record } = createTestContext("run_ai");
    const provider = createStraitProvider(ctx, { providerName: "openai" });

    const mockResult = {
      usage: { promptTokens: 100, completionTokens: 50 },
      toolCalls: [],
    };

    const result = await provider.wrapGenerate?.({
      doGenerate: async () => mockResult,
      params: {},
    });

    expect(result).toBe(mockResult);
    expect(record.usageReports).toHaveLength(1);
    expect(record.usageReports[0].provider).toBe("openai");
    expect(record.usageReports[0].promptTokens).toBe(100);
    expect(record.usageReports[0].completionTokens).toBe(50);
    expect(record.usageReports[0].totalTokens).toBe(150);
  });

  test("wrapGenerate logs tool calls", async () => {
    const { ctx, record } = createTestContext("run_ai");
    const provider = createStraitProvider(ctx, { providerName: "anthropic" });

    await provider.wrapGenerate?.({
      doGenerate: async () => ({
        usage: { promptTokens: 50, completionTokens: 25 },
        toolCalls: [
          { toolName: "search", args: { query: "test" } },
          { toolName: "calculate", args: { expression: "1+1" } },
        ],
      }),
      params: {},
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
      doGenerate: async () => ({
        usage: { promptTokens: 100, completionTokens: 50 },
        toolCalls: [{ toolName: "test", args: {} }],
      }),
      params: {},
    });

    expect(record.usageReports).toHaveLength(0);
    expect(record.toolCalls).toHaveLength(1);
  });

  test("wrapGenerate skips tool call logging when logToolCalls is false", async () => {
    const { ctx, record } = createTestContext("run_ai");
    const provider = createStraitProvider(ctx, { logToolCalls: false });

    await provider.wrapGenerate?.({
      doGenerate: async () => ({
        usage: { promptTokens: 100, completionTokens: 50 },
        toolCalls: [{ toolName: "test", args: {} }],
      }),
      params: {},
    });

    expect(record.usageReports).toHaveLength(1);
    expect(record.toolCalls).toHaveLength(0);
  });

  test("wrapStream forwards text chunks to ctx.streamChunk", async () => {
    const { ctx, record } = createTestContext("run_ai");
    const provider = createStraitProvider(ctx, { providerName: "openai" });

    const chunks = [
      { type: "text-delta", textDelta: "Hello" },
      { type: "text-delta", textDelta: " world" },
      { type: "finish", textDelta: undefined },
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
      doStream: async () => ({ stream: mockStream }),
      params: {},
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
        controller.enqueue({ type: "text-delta", textDelta: "Hello" });
        controller.close();
      },
    });

    // biome-ignore lint/style/noNonNullAssertion: test assertion
    const result = await provider.wrapStream!({
      doStream: async () => ({ stream: mockStream }),
      params: {},
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
