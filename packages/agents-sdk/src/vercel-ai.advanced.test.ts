import { describe, expect, it, vi } from "vitest";

import {
  autoBudget,
  budgetExceeded,
  provider,
  straitTelemetry,
  withStrait,
} from "./vercel-ai";

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

describe("vercel-ai advanced adapter surfaces", () => {
  it("wraps language models via middleware and reports streamed usage", async () => {
    const context = createContext();
    const model = withStrait(
      {
        specificationVersion: "v3",
        provider: "openai",
        modelId: "gpt-4.1",
        supportedUrls: {},
        doGenerate: async () => ({
          text: "done",
          finishReason: "stop",
          usage: {
            inputTokens: 10,
            outputTokens: 5,
            totalTokens: 15,
            inputTokenDetails: {
              noCacheTokens: 10,
              cacheReadTokens: 0,
              cacheWriteTokens: 0,
            },
            outputTokenDetails: {
              textTokens: 5,
              reasoningTokens: 0,
            },
          },
          warnings: [],
          response: {
            id: "resp-1",
            timestamp: new Date(),
            modelId: "gpt-4.1",
          },
        }),
        doStream: async () => ({
          stream: new ReadableStream({
            start(controller) {
              controller.enqueue({
                type: "text-delta",
                delta: "hel",
              });
              controller.enqueue({
                type: "finish",
                finishReason: "stop",
                rawFinishReason: "stop",
                usage: {
                  inputTokens: 10,
                  outputTokens: 5,
                  totalTokens: 15,
                  inputTokenDetails: {
                    noCacheTokens: 10,
                    cacheReadTokens: 0,
                    cacheWriteTokens: 0,
                  },
                  outputTokenDetails: {
                    textTokens: 5,
                    reasoningTokens: 0,
                  },
                },
              });
              controller.close();
            },
          }),
        }),
      } as never,
      {
        context,
        streamId: "assistant",
      }
    );

    await model.doGenerate({} as never);
    const streamResult = await model.doStream({} as never);
    await Array.fromAsync(streamResult.stream);

    expect(context.budgetExceeded).toHaveBeenCalledTimes(2);
    expect(context.reportUsage).toHaveBeenCalledWith(
      expect.objectContaining({
        provider: "openai",
        model: "gpt-4.1",
        totalTokens: 15,
      })
    );
    expect(context.stream).toHaveBeenCalledWith({
      chunk: "hel",
      streamId: "assistant",
    });
    expect(context.stream).toHaveBeenCalledWith({
      chunk: "",
      streamId: "assistant",
      done: true,
    });
  });

  it("provides stop conditions, auto-budget switching, and provider wrapping", async () => {
    const stopWhen = budgetExceeded("$0.00002");
    expect(
      stopWhen({
        steps: [createStepEvent(2, 2)] as never,
      })
    ).toBe(true);

    const fallbackModel = {
      specificationVersion: "v3",
      provider: "openai",
      modelId: "gpt-4.1",
      supportedUrls: {},
      doGenerate: async () => ({
        text: "done",
        finishReason: "stop",
        usage: {
          inputTokens: 10,
          outputTokens: 5,
          totalTokens: 15,
          inputTokenDetails: {
            noCacheTokens: 10,
            cacheReadTokens: 0,
            cacheWriteTokens: 0,
          },
          outputTokenDetails: {
            textTokens: 5,
            reasoningTokens: 0,
          },
        },
        warnings: [],
        response: {
          id: "resp-1",
          timestamp: new Date(),
          modelId: "gpt-4.1",
        },
      }),
      doStream: vi.fn(),
    };
    const context = createContext();
    const wrappedProvider = provider(
      {
        languageModel: vi.fn().mockReturnValue(fallbackModel),
      } as never,
      {
        context,
      }
    );

    const wrappedModel = wrappedProvider.languageModel("gpt-4.1") as any;
    await wrappedModel.doGenerate({} as never);

    const switched = await autoBudget({
      budget: "$5.00",
      above: "80%",
      switchTo: { provider: "openai", modelId: "gpt-4.1-mini" } as never,
      disableTools: ["search"],
    })({
      model: { provider: "openai", modelId: "gpt-4.1" },
      steps: [createStepEvent(1_000_000, 1_500_000)],
      activeTools: ["search", "write"],
    } as never);

    expect(switched).toEqual({
      model: { provider: "openai", modelId: "gpt-4.1-mini" },
      activeTools: ["write"],
    });
    expect(context.reportUsage).toHaveBeenCalled();
  });

  it("swallows telemetry integration failures", async () => {
    const telemetry = straitTelemetry({
      context: {
        checkpoint: vi.fn().mockRejectedValue(new Error("checkpoint failed")),
        reportToolCall: vi.fn().mockRejectedValue(new Error("tool failed")),
        reportUsage: vi.fn().mockRejectedValue(new Error("usage failed")),
        stream: vi.fn().mockRejectedValue(new Error("stream failed")),
      },
    });

    await expect(
      telemetry.onStepFinish?.(createStepEvent(10, 5) as never)
    ).resolves.toBeUndefined();
    await expect(
      telemetry.onToolCallFinish?.({
        toolCall: { toolName: "search", input: { query: "hello" } },
        success: true,
        output: { ok: true },
        durationMs: 25,
      } as never)
    ).resolves.toBeUndefined();
    await expect(telemetry.onFinish?.({} as never)).resolves.toBeUndefined();
  });
});
