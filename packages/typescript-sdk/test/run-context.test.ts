import { describe, expect, test } from "bun:test";

import {
  createRunContext,
  type RunContextClient,
} from "../src/authoring/run-context";

const createMockClient = () => {
  const calls: { method: string; args: unknown }[] = [];

  const track = (method: string) => (input: unknown) => {
    calls.push({ method, args: input });
    if (method === "spawnRun") {
      return Promise.resolve({ id: "spawn_1" });
    }
    if (method === "continueRun") {
      return Promise.resolve({ id: "continue_1" });
    }
    if (method === "waitForEventRun") {
      return Promise.resolve({
        status: "waiting",
        eventKey: "test",
        triggerId: "t_1",
        expiresAt: "2026-01-01T00:00:00Z",
      });
    }
    return Promise.resolve({});
  };

  const client: RunContextClient = {
    checkpointRun: track("checkpointRun"),
    heartbeatRun: track("heartbeatRun"),
    progressRun: track("progressRun"),
    logRun: track("logRun"),
    usageRun: track("usageRun"),
    toolCallRun: track("toolCallRun"),
    outputRun: track("outputRun"),
    waitForEventRun: track("waitForEventRun"),
    spawnRun: track("spawnRun"),
    continueRun: track("continueRun"),
    annotateRun: track("annotateRun"),
    completeRun: track("completeRun"),
    failRun: track("failRun"),
    stateRun: track("stateRun"),
    getStateByRunId: track("getStateByRunId"),
    getStateByRunIdAndKey: track("getStateByRunIdAndKey"),
    deleteState: track("deleteState"),
    streamRun: track("streamRun"),
  };

  return { client, calls };
};

describe("createRunContext", () => {
  test("sets runId, attempt, and signal", () => {
    const { client } = createMockClient();
    const signal = AbortSignal.timeout(5000);
    const ctx = createRunContext(client, "run_123", { attempt: 3, signal });

    expect(ctx.runId).toBe("run_123");
    expect(ctx.attempt).toBe(3);
    expect(ctx.signal).toBe(signal);
  });

  test("defaults attempt to 1", () => {
    const { client } = createMockClient();
    const ctx = createRunContext(client, "run_123");
    expect(ctx.attempt).toBe(1);
  });

  test("checkpoint sends state with source sdk", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.checkpoint({ step: 5 });

    expect(calls).toHaveLength(1);
    expect(calls[0].method).toBe("checkpointRun");
    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: { state: { step: 5 }, source: "sdk" },
    });
  });

  test("reportProgress sends percent and message", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.reportProgress(75, "Processing items");

    expect(calls[0].method).toBe("progressRun");
    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: { percent: 75, message: "Processing items" },
    });
  });

  test("heartbeat sends correct pathParams", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.heartbeat();

    expect(calls[0].method).toBe("heartbeatRun");
    expect(calls[0].args).toEqual({ pathParams: { runID: "run_123" } });
  });

  test("logger methods are fire-and-forget", () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    // These should not return promises
    const infoResult = ctx.logger.info("hello", { key: "value" });
    const warnResult = ctx.logger.warn("warning");
    const errorResult = ctx.logger.error("error");

    expect(infoResult).toBeUndefined();
    expect(warnResult).toBeUndefined();
    expect(errorResult).toBeUndefined();

    // But they should have triggered calls
    expect(calls.length).toBeGreaterThanOrEqual(3);
    expect(calls[0].method).toBe("logRun");
  });

  test("reportUsage maps camelCase to snake_case", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.reportUsage?.({
      provider: "openai",
      model: "gpt-4o",
      promptTokens: 100,
      completionTokens: 50,
      totalTokens: 150,
      costMicrousd: 500,
    });

    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: {
        provider: "openai",
        model: "gpt-4o",
        prompt_tokens: 100,
        completion_tokens: 50,
        total_tokens: 150,
        cost_microusd: 500,
      },
    });
  });

  test("logToolCall maps camelCase to snake_case", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.logToolCall?.({
      toolName: "search",
      input: { query: "test" },
      output: { results: [] },
      durationMs: 150,
      status: "success",
    });

    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: {
        tool_name: "search",
        input: { query: "test" },
        output: { results: [] },
        duration_ms: 150,
        status: "success",
      },
    });
  });

  test("saveOutput sends key, value, and schema", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.saveOutput?.("result", { total: 42 }, { type: "object" });

    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: { key: "result", value: { total: 42 }, schema: { type: "object" } },
    });
  });

  test("spawn sends job_slug not job_id", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    const result = await ctx.spawn?.({
      jobSlug: "my-job",
      projectId: "proj_1",
      payload: { key: "value" },
      priority: 5,
    });

    expect(result).toEqual({ id: "spawn_1" });
    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: {
        job_slug: "my-job",
        project_id: "proj_1",
        payload: { key: "value" },
        priority: 5,
      },
    });
  });

  test("continue sends payload", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    const result = await ctx.continue?.({ next: "step" });

    expect(result).toEqual({ id: "continue_1" });
    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: { payload: { next: "step" } },
    });
  });

  test("waitForEvent maps to snake_case", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.waitForEvent?.("payment.completed", { timeoutSecs: 60 });

    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: {
        event_key: "payment.completed",
        timeout_secs: 60,
        notify_url: undefined,
      },
    });
  });

  test("annotate sends annotations", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.annotate?.({ team: "backend" });

    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: { annotations: { team: "backend" } },
    });
  });

  test("complete and fail send correct bodies", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.complete?.({ output: "done" });
    await ctx.fail?.("something broke");

    expect(calls[0].method).toBe("completeRun");
    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: { result: { output: "done" } },
    });
    expect(calls[1].method).toBe("failRun");
    expect(calls[1].args).toEqual({
      pathParams: { runID: "run_123" },
      body: { error: "something broke" },
    });
  });

  test("state.set sends key and value via stateRun", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.state?.set("counter", 42);

    expect(calls[0].method).toBe("stateRun");
    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: { key: "counter", value: 42 },
    });
  });

  test("state.get calls getStateByRunIdAndKey with key path param", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.state?.get("counter");

    expect(calls[0].method).toBe("getStateByRunIdAndKey");
    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123", key: "counter" },
    });
  });

  test("state.delete calls deleteState with key path param", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.state?.delete("counter");

    expect(calls[0].method).toBe("deleteState");
    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123", key: "counter" },
    });
  });

  test("state.list calls getStateByRunId", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.state?.list();

    expect(calls[0].method).toBe("getStateByRunId");
    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
    });
  });

  test("streamChunk calls streamRun with correct body", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_123");

    await ctx.streamChunk?.("Hello world", { streamId: "s1", done: false });

    expect(calls[0].method).toBe("streamRun");
    expect(calls[0].args).toEqual({
      pathParams: { runID: "run_123" },
      body: { chunk: "Hello world", stream_id: "s1", done: false },
    });
  });

  test("all methods pass correct runID in pathParams", async () => {
    const { client, calls } = createMockClient();
    const ctx = createRunContext(client, "run_abc");

    await ctx.checkpoint({ x: 1 });
    await ctx.heartbeat();
    await ctx.reportProgress(50);
    await ctx.reportUsage?.({ provider: "test", model: "test" });
    await ctx.logToolCall?.({ toolName: "test" });
    await ctx.saveOutput?.("k", { v: 1 });

    for (const call of calls) {
      expect(
        (call.args as { pathParams: { runID: string } }).pathParams.runID
      ).toBe("run_abc");
    }
  });
});
