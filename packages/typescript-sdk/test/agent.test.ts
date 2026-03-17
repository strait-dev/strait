import { describe, expect, test } from "bun:test";

import { defineAgent } from "../src/authoring/agent";
import { zodSchema } from "../src/index";

const mockSchema = zodSchema({
  parse: (input: unknown) => input as { prompt: string },
  toJSON: () => ({ type: "object" }),
});

describe("defineAgent", () => {
  test("produces a valid job definition", () => {
    const agent = defineAgent({
      name: "Test Agent",
      slug: "test-agent",
      endpointUrl: "https://example.com/agent",
      schema: mockSchema,
      run: async (payload) => ({ response: payload.prompt }),
    });

    expect(agent.kind).toBe("job");
    expect(agent.slug).toBe("test-agent");
    expect(agent.run).toBeDefined();
  });

  test("tags include strait.kind: agent", () => {
    const agent = defineAgent({
      name: "Test Agent",
      slug: "test-agent",
      endpointUrl: "https://example.com/agent",
      schema: mockSchema,
      projectId: "proj_1",
      tags: { team: "ml" },
      run: async () => ({}),
    });

    const body = agent.toRegistrationBody();
    expect(body.tags).toEqual({ team: "ml", "strait.kind": "agent" });
  });

  test("has higher default timeout", () => {
    const agent = defineAgent({
      name: "Test Agent",
      slug: "test-agent",
      endpointUrl: "https://example.com/agent",
      schema: mockSchema,
      projectId: "proj_1",
      run: async () => ({}),
    });

    const body = agent.toRegistrationBody();
    expect(body.timeout_secs).toBe(600);
  });

  test("has exponential retry by default", () => {
    const agent = defineAgent({
      name: "Test Agent",
      slug: "test-agent",
      endpointUrl: "https://example.com/agent",
      schema: mockSchema,
      projectId: "proj_1",
      run: async () => ({}),
    });

    const body = agent.toRegistrationBody();
    expect(body.retry_strategy).toBe("exponential");
    expect(body.max_attempts).toBe(5);
  });

  test("AgentRunContext tracks cost accumulation", async () => {
    let trackedCost = 0;
    let budgetExceeded = false;

    const agent = defineAgent({
      name: "Cost Agent",
      slug: "cost-agent",
      endpointUrl: "https://example.com/agent",
      schema: mockSchema,
      maxCostMicrousd: 1000,
      run: async (_payload, ctx) => {
        await ctx.reportUsage?.({
          provider: "openai",
          model: "gpt-4o",
          costMicrousd: 300,
        });
        trackedCost = ctx.accumulatedCostMicrousd();
        await ctx.reportUsage?.({
          provider: "openai",
          model: "gpt-4o",
          costMicrousd: 800,
        });
        budgetExceeded = ctx.isBudgetExceeded();
        return {};
      },
    });

    await agent.run?.(
      { prompt: "test" },
      {
        runId: "run_test",
        attempt: 1,
        signal: AbortSignal.timeout(5000),
        // biome-ignore lint/suspicious/noEmptyBlockStatements: noop stub
        logger: { info: () => {}, warn: () => {}, error: () => {} },
        checkpoint: () => Promise.resolve(),
        reportProgress: () => Promise.resolve(),
        heartbeat: () => Promise.resolve(),
        reportUsage: () => Promise.resolve(),
      }
    );

    expect(trackedCost).toBe(300);
    expect(budgetExceeded).toBe(true);
  });

  test("isBudgetExceeded returns false when no budget set", async () => {
    let exceeded = true;

    const agent = defineAgent({
      name: "No Budget Agent",
      slug: "no-budget",
      endpointUrl: "https://example.com/agent",
      schema: mockSchema,
      run: async (_payload, ctx) => {
        await ctx.reportUsage?.({
          provider: "openai",
          model: "gpt-4o",
          costMicrousd: 999_999,
        });
        exceeded = ctx.isBudgetExceeded();
        return {};
      },
    });

    await agent.run?.(
      { prompt: "test" },
      {
        runId: "run_test",
        attempt: 1,
        signal: AbortSignal.timeout(5000),
        // biome-ignore lint/suspicious/noEmptyBlockStatements: noop stub
        logger: { info: () => {}, warn: () => {}, error: () => {} },
        checkpoint: () => Promise.resolve(),
        reportProgress: () => Promise.resolve(),
        heartbeat: () => Promise.resolve(),
        reportUsage: () => Promise.resolve(),
      }
    );

    expect(exceeded).toBe(false);
  });

  test("iteration increments on checkpoint", async () => {
    let lastIteration = -1;

    const agent = defineAgent({
      name: "Iteration Agent",
      slug: "iteration-agent",
      endpointUrl: "https://example.com/agent",
      schema: mockSchema,
      run: async (_payload, ctx) => {
        expect(ctx.iteration).toBe(0);
        await ctx.checkpoint({ step: 1 });
        expect(ctx.iteration).toBe(1);
        await ctx.checkpoint({ step: 2 });
        lastIteration = ctx.iteration;
        return {};
      },
    });

    await agent.run?.(
      { prompt: "test" },
      {
        runId: "run_test",
        attempt: 1,
        signal: AbortSignal.timeout(5000),
        // biome-ignore lint/suspicious/noEmptyBlockStatements: noop stub
        logger: { info: () => {}, warn: () => {}, error: () => {} },
        checkpoint: () => Promise.resolve(),
        reportProgress: () => Promise.resolve(),
        heartbeat: () => Promise.resolve(),
      }
    );

    expect(lastIteration).toBe(2);
  });
});
