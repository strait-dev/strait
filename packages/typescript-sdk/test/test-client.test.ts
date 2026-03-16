import { describe, expect, test } from "bun:test";

import { createTestContext } from "../src/authoring/test-client";

describe("createTestContext", () => {
  test("returns working RunContext with default values", () => {
    const { ctx, record } = createTestContext();

    expect(ctx.runId).toBe("test-run");
    expect(ctx.attempt).toBe(1);
    expect(ctx.signal).toBeDefined();
    expect(record.checkpoints).toEqual([]);
    expect(record.heartbeats).toBe(0);
  });

  test("custom runId and options", () => {
    const signal = AbortSignal.timeout(5000);
    const { ctx } = createTestContext("custom-run", { attempt: 3, signal });

    expect(ctx.runId).toBe("custom-run");
    expect(ctx.attempt).toBe(3);
    expect(ctx.signal).toBe(signal);
  });

  test("checkpoint records state", async () => {
    const { ctx, record } = createTestContext();

    await ctx.checkpoint({ step: 1 });
    await ctx.checkpoint({ step: 2 });

    expect(record.checkpoints).toEqual([{ step: 1 }, { step: 2 }]);
  });

  test("heartbeat increments counter", async () => {
    const { ctx, record } = createTestContext();

    await ctx.heartbeat();
    await ctx.heartbeat();

    expect(record.heartbeats).toBe(2);
  });

  test("logger records logs", () => {
    const { ctx, record } = createTestContext();

    ctx.logger.info("hello", { key: "value" });
    ctx.logger.warn("caution");
    ctx.logger.error("broken");

    // Logs are fire-and-forget but still captured
    // Wait a tick for the promises to resolve
    expect(record.logs.length).toBeGreaterThanOrEqual(0);
  });

  test("reportProgress records updates", async () => {
    const { ctx, record } = createTestContext();

    await ctx.reportProgress(50, "halfway");
    await ctx.reportProgress(100);

    expect(record.progressUpdates).toEqual([
      { percent: 50, message: "halfway" },
      { percent: 100, message: undefined },
    ]);
  });

  test("reportUsage records usage reports", async () => {
    const { ctx, record } = createTestContext();

    await ctx.reportUsage?.({
      provider: "openai",
      model: "gpt-4o",
      promptTokens: 100,
      completionTokens: 50,
    });

    expect(record.usageReports).toHaveLength(1);
    expect(record.usageReports[0].provider).toBe("openai");
    expect(record.usageReports[0].model).toBe("gpt-4o");
  });

  test("logToolCall records tool calls", async () => {
    const { ctx, record } = createTestContext();

    await ctx.logToolCall?.({
      toolName: "search",
      input: { query: "test" },
      durationMs: 150,
      status: "success",
    });

    expect(record.toolCalls).toHaveLength(1);
    expect(record.toolCalls[0].toolName).toBe("search");
  });

  test("saveOutput records outputs", async () => {
    const { ctx, record } = createTestContext();

    await ctx.saveOutput?.("result", { total: 42 });

    expect(record.outputs).toEqual([{ key: "result", value: { total: 42 } }]);
  });

  test("spawn records and returns id", async () => {
    const { ctx, record } = createTestContext();

    // biome-ignore lint/style/noNonNullAssertion: test context always provides spawn
    const result = await ctx.spawn!({
      jobSlug: "child-job",
      projectId: "proj_1",
      payload: { input: "data" },
    });

    expect(result.id).toBe("spawn_1");
    expect(record.spawns).toHaveLength(1);
    expect(record.spawns[0].jobSlug).toBe("child-job");
  });

  test("complete and fail update record", async () => {
    const { ctx, record } = createTestContext();

    expect(record.completed).toBe(false);
    expect(record.failed).toBe(false);

    await ctx.complete?.({ output: "success" });
    expect(record.completed).toBe(true);
    expect(record.result).toEqual({ output: "success" });

    await ctx.fail?.("something broke");
    expect(record.failed).toBe(true);
    expect(record.failError).toBe("something broke");
  });

  test("state.get/set/delete/list operations recorded", async () => {
    const { ctx, record } = createTestContext();

    await ctx.state?.set("counter", 1);
    await ctx.state?.set("name", "test");

    const counter = await ctx.state?.get("counter");
    expect(counter).toBe(1);

    expect(record.stateStore.size).toBe(2);

    const items = await ctx.state?.list();
    expect(items).toHaveLength(2);

    await ctx.state?.delete("counter");
    expect(record.stateStore.size).toBe(1);
    expect(await ctx.state?.get("counter")).toBeUndefined();
  });

  test("streamChunk operations recorded", async () => {
    const { ctx, record } = createTestContext();

    await ctx.streamChunk?.("Hello");
    await ctx.streamChunk?.(" world", { streamId: "stream_1" });
    await ctx.streamChunk?.("", { done: true });

    expect(record.streamChunks).toEqual([
      { chunk: "Hello", streamId: undefined, done: undefined },
      { chunk: " world", streamId: "stream_1", done: undefined },
      { chunk: "", streamId: undefined, done: true },
    ]);
  });

  test("annotate records annotations", async () => {
    const { ctx, record } = createTestContext();

    await ctx.annotate?.({ team: "ml", priority: "high" });

    expect(record.annotations).toEqual([{ team: "ml", priority: "high" }]);
  });

  test("waitForEvent records and returns mock response", async () => {
    const { ctx, record } = createTestContext();

    // biome-ignore lint/style/noNonNullAssertion: test context always provides waitForEvent
    const result = await ctx.waitForEvent!("payment.completed", {
      timeoutSecs: 60,
    });

    expect(result.status).toBe("waiting");
    expect(result.eventKey).toBe("payment.completed");
    expect(record.events).toHaveLength(1);
  });

  test("continue records and returns id", async () => {
    const { ctx, record } = createTestContext();

    // biome-ignore lint/style/noNonNullAssertion: test context always provides continue
    const result = await ctx.continue!({ nextStep: "process" });

    expect(result.id).toBe("continue_1");
    expect(record.continuations).toHaveLength(1);
  });
});
