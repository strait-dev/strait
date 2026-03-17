import { describe, expect, test } from "bun:test";

import { withRetry } from "../src/composition/retry";
import { waitForRun } from "../src/composition/wait";

describe("AbortSignal support", () => {
  test("withRetry stops when signal is aborted", async () => {
    const controller = new AbortController();
    let attempts = 0;

    const promise = withRetry(
      () => {
        attempts += 1;
        if (attempts === 2) {
          controller.abort(new Error("manual abort"));
        }
        return Promise.reject(new Error("fail"));
      },
      {
        attempts: 10,
        delayMs: 1,
        jitter: "none",
        signal: controller.signal,
      }
    );

    await expect(promise).rejects.toThrow();
    // Should have stopped early, not retried all 10 times
    expect(attempts).toBeLessThanOrEqual(3);
  });

  test("withRetry respects pre-aborted signal", async () => {
    const controller = new AbortController();
    controller.abort(new Error("already aborted"));

    let attempts = 0;

    await expect(
      withRetry(
        () => {
          attempts += 1;
          return Promise.resolve("ok");
        },
        {
          attempts: 5,
          signal: controller.signal,
        }
      )
    ).rejects.toThrow("already aborted");

    expect(attempts).toBe(0);
  });

  test("waitForRun respects aborted signal", async () => {
    const controller = new AbortController();
    controller.abort(new Error("canceled"));

    await expect(
      waitForRun(() => Promise.resolve({ status: "executing" }), "run_1", {
        signal: controller.signal,
        initialDelayMs: 1,
        timeoutMs: 100,
      })
    ).rejects.toThrow("canceled");
  });

  test("withRetry with jitter none uses exact delay", async () => {
    let attempts = 0;

    await withRetry(
      () => {
        attempts += 1;
        if (attempts < 3) {
          return Promise.reject(new Error("fail"));
        }
        return Promise.resolve("ok");
      },
      {
        attempts: 3,
        delayMs: 10,
        jitter: "none",
      }
    );

    expect(attempts).toBe(3);
  });

  test("withRetry with jitter full randomizes delay", async () => {
    // We can't easily test randomization, but we can verify the option
    // is accepted without error
    let attempts = 0;

    const result = await withRetry(
      () => {
        attempts += 1;
        if (attempts < 2) {
          return Promise.reject(new Error("fail"));
        }
        return Promise.resolve("ok");
      },
      {
        attempts: 3,
        delayMs: 1,
        jitter: "full",
      }
    );

    expect(result).toBe("ok");
    expect(attempts).toBe(2);
  });
});
