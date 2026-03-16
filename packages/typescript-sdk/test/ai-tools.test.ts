import { describe, expect, test } from "bun:test";
import { createStraitTools } from "../src/ai/tools";
import { createTestContext } from "../src/authoring/test-client";

describe("createStraitTools", () => {
  test("creates default tool set", () => {
    const { ctx } = createTestContext();
    const tools = createStraitTools(ctx);

    expect(tools.strait_checkpoint).toBeDefined();
    expect(tools.strait_spawn).toBeDefined();
    expect(tools.strait_save_output).toBeDefined();
    expect(tools.strait_wait_for_event).toBeDefined();
    expect(tools.strait_state_get).toBeDefined();
    expect(tools.strait_state_set).toBeDefined();
    // complete is off by default
    expect(tools.strait_complete).toBeUndefined();
  });

  test("each tool has description, inputSchema, and execute", () => {
    const { ctx } = createTestContext();
    const tools = createStraitTools(ctx);

    for (const [_name, t] of Object.entries(tools)) {
      expect(t.description).toBeTruthy();
      expect(t.inputSchema).toBeDefined();
      expect(t.execute).toBeInstanceOf(Function);
    }
  });

  test("strait_checkpoint executes correctly", async () => {
    const { ctx, record } = createTestContext();
    const tools = createStraitTools(ctx);

    // biome-ignore lint/style/noNonNullAssertion: test assertion
    const result = await tools.strait_checkpoint.execute!(
      { state: { step: 3 } },
      { toolCallId: "tc_1", messages: [], abortSignal: undefined as never }
    );

    expect(result).toEqual({ success: true });
    expect(record.checkpoints).toHaveLength(1);
    expect(record.checkpoints[0]).toEqual({ step: 3 });
  });

  test("strait_spawn executes correctly", async () => {
    const { ctx, record } = createTestContext();
    const tools = createStraitTools(ctx);

    // biome-ignore lint/style/noNonNullAssertion: test assertion
    const result = await tools.strait_spawn.execute!(
      {
        jobSlug: "process-data",
        projectId: "proj_1",
        payload: { items: [1, 2, 3] },
      },
      { toolCallId: "tc_2", messages: [], abortSignal: undefined as never }
    );

    expect(result).toEqual({ id: "spawn_1" });
    expect(record.spawns).toHaveLength(1);
    expect(record.spawns[0].jobSlug).toBe("process-data");
  });

  test("strait_save_output executes correctly", async () => {
    const { ctx, record } = createTestContext();
    const tools = createStraitTools(ctx);

    // biome-ignore lint/style/noNonNullAssertion: test assertion
    await tools.strait_save_output.execute!(
      {
        key: "summary",
        value: { text: "Done" },
      },
      { toolCallId: "tc_3", messages: [], abortSignal: undefined as never }
    );

    expect(record.outputs).toHaveLength(1);
    expect(record.outputs[0].key).toBe("summary");
  });

  test("strait_wait_for_event executes correctly", async () => {
    const { ctx, record } = createTestContext();
    const tools = createStraitTools(ctx);

    // biome-ignore lint/style/noNonNullAssertion: test assertion
    const result = await tools.strait_wait_for_event.execute!(
      {
        eventKey: "approval.granted",
        timeoutSecs: 300,
      },
      { toolCallId: "tc_4", messages: [], abortSignal: undefined as never }
    );

    expect(result).toHaveProperty("status");
    expect(record.events).toHaveLength(1);
    expect(record.events[0].eventKey).toBe("approval.granted");
  });

  test("strait_state_get and strait_state_set work together", async () => {
    const { ctx, record } = createTestContext();
    const tools = createStraitTools(ctx);

    // biome-ignore lint/style/noNonNullAssertion: test assertion
    await tools.strait_state_set.execute!(
      { key: "counter", value: 42 },
      { toolCallId: "tc_5", messages: [], abortSignal: undefined as never }
    );
    // biome-ignore lint/style/noNonNullAssertion: test assertion
    const result = await tools.strait_state_get.execute!(
      { key: "counter" },
      { toolCallId: "tc_6", messages: [], abortSignal: undefined as never }
    );

    expect(result).toEqual({ key: "counter", value: 42 });
    expect(record.stateStore.get("counter")).toBe(42);
  });

  test("complete tool is opt-in", () => {
    const { ctx } = createTestContext();

    const toolsDefault = createStraitTools(ctx);
    expect(toolsDefault.strait_complete).toBeUndefined();

    const toolsWithComplete = createStraitTools(ctx, { complete: true });
    expect(toolsWithComplete.strait_complete).toBeDefined();
  });

  test("tools can be disabled via options", () => {
    const { ctx } = createTestContext();
    const tools = createStraitTools(ctx, {
      checkpoint: false,
      spawn: false,
      saveOutput: false,
    });

    expect(tools.strait_checkpoint).toBeUndefined();
    expect(tools.strait_spawn).toBeUndefined();
    expect(tools.strait_save_output).toBeUndefined();
    expect(tools.strait_wait_for_event).toBeDefined();
    expect(tools.strait_state_get).toBeDefined();
  });
});
