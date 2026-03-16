import { describe, expect, test } from "bun:test";
import { jsonSchema, type LanguageModel, ToolLoopAgent, tool } from "ai";
import { createStraitAgent } from "../src/ai/agent";
import { createTestContext } from "../src/authoring/test-client";

const mockModel = {
  specificationVersion: "v3" as const,
  provider: "test",
  modelId: "test-model",
  defaultObjectGenerationMode: "json" as const,
  supportsImageUrls: false,
  supportedUrls: {},
  supportsUrl: async () => false,
  doGenerate: async () => ({
    content: [],
    finishReason: "stop" as const,
    warnings: [],
    usage: {
      inputTokens: {
        total: 0,
        noCache: undefined,
        cacheRead: undefined,
        cacheWrite: undefined,
      },
      outputTokens: {
        total: 0,
        noCache: undefined,
        cacheRead: undefined,
        cacheWrite: undefined,
      },
    },
  }),
  doStream: async () => ({
    stream: new ReadableStream(),
  }),
} as unknown as LanguageModel;

describe("createStraitAgent", () => {
  test("returns a ToolLoopAgent instance", () => {
    const { ctx } = createTestContext();
    const agent = createStraitAgent(ctx, {
      model: mockModel,
      instructions: "You are a helpful assistant.",
    });

    expect(agent).toBeInstanceOf(ToolLoopAgent);
  });

  test("agent has strait tools available", () => {
    const { ctx } = createTestContext();
    const agent = createStraitAgent(ctx, {
      model: mockModel,
      instructions: "You are a helpful assistant.",
    });

    const tools = agent.tools as Record<string, unknown>;
    expect(tools.strait_checkpoint).toBeDefined();
    expect(tools.strait_spawn).toBeDefined();
    expect(tools.strait_save_output).toBeDefined();
  });

  test("agent merges custom tools with strait tools", () => {
    const { ctx } = createTestContext();

    const customTool = tool({
      description: "A custom tool",
      inputSchema: jsonSchema<{ query: string }>({
        type: "object",
        properties: { query: { type: "string" } },
        required: ["query"],
      }),
      execute: async ({ query }) => ({ result: query }),
    });

    const agent = createStraitAgent(ctx, {
      model: mockModel,
      instructions: "You are a helpful assistant.",
      tools: { my_tool: customTool },
    });

    const tools = agent.tools as Record<string, unknown>;
    expect(tools.strait_checkpoint).toBeDefined();
    expect(tools.my_tool).toBeDefined();
  });

  test("agent respects strait tool options", () => {
    const { ctx } = createTestContext();
    const agent = createStraitAgent(ctx, {
      model: mockModel,
      instructions: "You are a helpful assistant.",
      straitTools: {
        checkpoint: false,
        spawn: false,
      },
    });

    const tools = agent.tools as Record<string, unknown>;
    expect(tools.strait_checkpoint).toBeUndefined();
    expect(tools.strait_spawn).toBeUndefined();
    expect(tools.strait_save_output).toBeDefined();
  });
});
