import { describe, expect, test } from "bun:test";
import { createClient } from "../src/client";
import { withRetry } from "../src/composition/retry";
import { waitForRun } from "../src/composition/wait";
import { TimeoutError } from "../src/errors";
import { customSchema, defineJob, zodSchema } from "../src/index";
import type { FetchLike } from "../src/runtime";

describe("withRetry error paths", () => {
  test("throws last error when retries exhausted", async () => {
    let attempt = 0;

    await expect(
      withRetry(
        () => {
          attempt += 1;
          return Promise.reject(new Error(`fail-${attempt}`));
        },
        { attempts: 3, delayMs: 1, jitter: "none" }
      )
    ).rejects.toThrow("fail-3");

    expect(attempt).toBe(3);
  });

  test("stops retrying when shouldRetry returns false", async () => {
    let attempt = 0;

    await expect(
      withRetry(
        () => {
          attempt += 1;
          return Promise.reject(new Error("permanent"));
        },
        {
          attempts: 10,
          delayMs: 1,
          jitter: "none",
          shouldRetry: (_err, ctx) => ctx.attempt < 2,
        }
      )
    ).rejects.toThrow("permanent");

    expect(attempt).toBe(2);
  });

  test("does not retry when attempts is 1", async () => {
    let attempt = 0;

    await expect(
      withRetry(
        () => {
          attempt += 1;
          return Promise.reject(new Error("once"));
        },
        { attempts: 1 }
      )
    ).rejects.toThrow("once");

    expect(attempt).toBe(1);
  });

  test("succeeds on first try when operation works", async () => {
    const result = await withRetry(() => Promise.resolve("ok"), {
      attempts: 3,
    });
    expect(result).toBe("ok");
  });
});

describe("waitForRun error paths", () => {
  test("TimeoutError includes runId and elapsedMs", async () => {
    try {
      await waitForRun(
        () => Promise.resolve({ status: "executing" }),
        "run_timeout_test",
        { timeoutMs: 10, initialDelayMs: 1, maxDelayMs: 2 }
      );
      expect.unreachable("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(TimeoutError);
      const err = e as TimeoutError;
      expect(err.runId).toBe("run_timeout_test");
      expect(err.elapsedMs).toBeGreaterThan(0);
    }
  });

  test("propagates getRun errors", async () => {
    await expect(
      waitForRun(
        () => Promise.reject(new Error("network failure")),
        "run_err",
        { timeoutMs: 100, initialDelayMs: 1 }
      )
    ).rejects.toThrow("network failure");
  });

  test("custom isTerminal predicate is respected", async () => {
    const statuses = ["initializing", "custom_done"];
    let idx = 0;

    const result = await waitForRun(
      () => {
        const status = statuses[idx] ?? "custom_done";
        idx += 1;
        return Promise.resolve({ status });
      },
      "run_custom",
      {
        initialDelayMs: 1,
        maxDelayMs: 2,
        timeoutMs: 100,
        isTerminal: (s) => s === "custom_done",
      }
    );

    expect(result.status).toBe("custom_done");
  });
});

describe("middleware error propagation", () => {
  test("onRequest exception surfaces as error to caller", async () => {
    const fetchImpl: FetchLike = () =>
      Promise.resolve(new Response(JSON.stringify({}), { status: 200 }));

    const client = createClient(
      {
        baseUrl: "https://api.test.io",
        auth: { type: "bearer", token: "tok" },
      },
      {
        fetch: fetchImpl,
        middleware: [
          {
            onRequest: () => {
              throw new Error("middleware exploded");
            },
          },
        ],
      }
    );

    // onRequest throws before fetch, the middleware-wrapped fetch rejects,
    // which surfaces as a TransportError through the Effect pipeline.
    await expect(client.operationsPromise.getHealth()).rejects.toThrow();
  });

  test("onResponse exception is caught and surfaces as error", async () => {
    const fetchImpl: FetchLike = () =>
      Promise.resolve(new Response(JSON.stringify({}), { status: 200 }));

    const client = createClient(
      {
        baseUrl: "https://api.test.io",
        auth: { type: "bearer", token: "tok" },
      },
      {
        fetch: fetchImpl,
        middleware: [
          {
            onResponse: () => {
              throw new Error("response hook failed");
            },
          },
        ],
      }
    );

    // onResponse throws after fetch completes, wrapping the response.
    // The middleware-wrapped fetch rejects, which becomes a TransportError.
    await expect(client.operationsPromise.getHealth()).rejects.toThrow();
  });
});

describe("schema adapter error paths", () => {
  test("zodSchema parse failure propagates to trigger", async () => {
    const job = defineJob({
      name: "Schema Fail",
      slug: "schema-fail",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: zodSchema({
        parse: () => {
          throw new Error("zod validation failed");
        },
      }),
    });

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    await expect(
      job.trigger(
        {
          createJob: () => Promise.resolve({ id: "job_1" }),
          triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
        },
        { payload: { bad: "data" } as unknown as never }
      )
    ).rejects.toThrow("zod validation failed");
  });

  test("customSchema parse failure propagates to trigger", async () => {
    const job = defineJob({
      name: "Custom Fail",
      slug: "custom-fail",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: customSchema(() => {
        throw new Error("custom validation failed");
      }),
    });

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    await expect(
      job.trigger(
        {
          createJob: () => Promise.resolve({ id: "job_1" }),
          triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
        },
        { payload: null as never }
      )
    ).rejects.toThrow("custom validation failed");
  });

  test("customSchema with async parse works", async () => {
    const schema = customSchema<{ id: string }>(async (input) => {
      await Promise.resolve();
      if (typeof input !== "object" || input === null || !("id" in input)) {
        throw new Error("missing id");
      }
      return input as { id: string };
    });

    const parsed = await schema.parse({ id: "abc" });
    expect(parsed).toEqual({ id: "abc" });

    await expect(schema.parse({})).rejects.toThrow("missing id");
  });

  test("customSchema with toJsonSchema returns schema", () => {
    const schema = customSchema<{ id: string }>(
      (input) => input as { id: string },
      {
        toJsonSchema: () => ({
          type: "object",
          properties: { id: { type: "string" } },
        }),
      }
    );

    expect(schema.toJsonSchema?.()).toEqual({
      type: "object",
      properties: { id: { type: "string" } },
    });
  });

  test("customSchema without toJsonSchema returns undefined", () => {
    const schema = customSchema<string>((input) => String(input));
    expect(schema.toJsonSchema).toBeUndefined();
  });
});

describe("node env config", () => {
  test("createClientFromEnv throws when STRAIT_BASE_URL is missing", async () => {
    const { createClientFromEnv } = await import("../src/node/config");

    const originalBaseUrl = process.env.STRAIT_BASE_URL;
    const originalApiKey = process.env.STRAIT_API_KEY;

    try {
      process.env.STRAIT_BASE_URL = undefined;
      process.env.STRAIT_API_KEY = "test_key";

      expect(() => createClientFromEnv()).toThrow("STRAIT_BASE_URL");
    } finally {
      process.env.STRAIT_BASE_URL = originalBaseUrl;
      process.env.STRAIT_API_KEY = originalApiKey;
    }
  });

  test("createClientFromEnv throws when STRAIT_API_KEY is missing", async () => {
    const { createClientFromEnv } = await import("../src/node/config");

    const originalBaseUrl = process.env.STRAIT_BASE_URL;
    const originalApiKey = process.env.STRAIT_API_KEY;

    try {
      process.env.STRAIT_BASE_URL = "https://api.test.io";
      process.env.STRAIT_API_KEY = undefined;

      expect(() => createClientFromEnv()).toThrow("STRAIT_API_KEY");
    } finally {
      process.env.STRAIT_BASE_URL = originalBaseUrl;
      process.env.STRAIT_API_KEY = originalApiKey;
    }
  });

  test("createClientFromEnv creates client with env vars", async () => {
    const { createClientFromEnv } = await import("../src/node/config");

    const originalBaseUrl = process.env.STRAIT_BASE_URL;
    const originalApiKey = process.env.STRAIT_API_KEY;

    try {
      process.env.STRAIT_BASE_URL = "https://api.test.io";
      process.env.STRAIT_API_KEY = "tok_from_env";

      const client = createClientFromEnv();
      expect(client).toBeDefined();
      expect(client.operationsPromise).toBeDefined();
    } finally {
      process.env.STRAIT_BASE_URL = originalBaseUrl;
      process.env.STRAIT_API_KEY = originalApiKey;
    }
  });
});
