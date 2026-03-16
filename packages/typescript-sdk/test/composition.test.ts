import { describe, expect, test } from "bun:test";

import {
  err,
  fromPromise,
  isErr,
  isOk,
  waitForRun,
  withIdempotency,
  withRetry,
} from "../src/composition/index";

describe("composition helpers", () => {
  test("withIdempotency injects idempotency header", () => {
    const result = withIdempotency(
      {
        headers: {
          "X-Custom": "value",
        },
      },
      "idem_123"
    );

    expect(result.headers).toEqual({
      "X-Custom": "value",
      "Idempotency-Key": "idem_123",
    });
  });

  test("withRetry retries until operation succeeds", async () => {
    let attempts = 0;

    const value = await withRetry(
      () => {
        attempts += 1;
        if (attempts < 3) {
          return Promise.reject(new Error("retry me"));
        }

        return Promise.resolve("ok");
      },
      {
        attempts: 3,
        delayMs: 1,
      }
    );

    expect(value).toBe("ok");
    expect(attempts).toBe(3);
  });

  test("fromPromise returns typed result with unwrap ergonomics", async () => {
    const success = await fromPromise(() => Promise.resolve({ id: "run_1" }));
    const failure = await fromPromise(() => Promise.reject(new Error("boom")));

    expect(isOk(success)).toBe(true);
    expect(isErr(failure)).toBe(true);
    expect(success.unwrap()).toEqual({ id: "run_1" });
    expect(() => failure.unwrap()).toThrow();

    const explicitFailure = err("nope");
    expect(explicitFailure.ok).toBe(false);
    expect(() => explicitFailure.unwrap()).toThrow("nope");
  });

  test("waitForRun polls until terminal status", async () => {
    const statuses = ["queued", "executing", "completed"];
    let index = 0;

    const run = await waitForRun(
      () => {
        const status = statuses[index] ?? "completed";
        index += 1;
        return Promise.resolve({
          status,
          index,
        });
      },
      "run_123",
      {
        initialDelayMs: 1,
        maxDelayMs: 2,
        timeoutMs: 50,
      }
    );

    expect(run.status).toBe("completed");
    expect(index).toBe(3);
  });
});
