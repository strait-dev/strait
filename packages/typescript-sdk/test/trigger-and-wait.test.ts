import { describe, expect, test } from "bun:test";

import { triggerAndWait } from "../src/composition/trigger-and-wait";
import { TimeoutError } from "../src/errors";

describe("standalone triggerAndWait", () => {
  test("triggers and waits for terminal status", async () => {
    const statuses = ["queued", "executing", "completed"];
    let pollIndex = 0;

    const result = await triggerAndWait(
      (_input: { payload: string }) =>
        Promise.resolve({ id: "run_1", status: "queued" }),
      () => {
        const status = statuses[pollIndex] ?? "completed";
        pollIndex += 1;
        return Promise.resolve({ id: "run_1", status });
      },
      { payload: "test" },
      { initialDelayMs: 1, maxDelayMs: 2, timeoutMs: 100 }
    );

    expect(result.status).toBe("completed");
  });

  test("times out with TimeoutError", async () => {
    try {
      await triggerAndWait(
        () => Promise.resolve({ id: "run_1", status: "queued" }),
        () => Promise.resolve({ id: "run_1", status: "executing" }),
        {},
        { timeoutMs: 10, initialDelayMs: 1, maxDelayMs: 2 }
      );
      expect.unreachable("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(TimeoutError);
      expect((e as TimeoutError).runId).toBe("run_1");
    }
  });

  test("passes trigger input to trigger function", async () => {
    let capturedInput: unknown;

    await triggerAndWait(
      (input: { payload: string }) => {
        capturedInput = input;
        return Promise.resolve({ id: "run_1", status: "completed" });
      },
      () => Promise.resolve({ id: "run_1", status: "completed" }),
      { payload: "hello" },
      { initialDelayMs: 1, maxDelayMs: 2, timeoutMs: 50 }
    );

    expect(capturedInput).toEqual({ payload: "hello" });
  });
});
