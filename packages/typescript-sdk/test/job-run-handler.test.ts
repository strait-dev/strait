import { describe, expect, test } from "bun:test";

import { defineJob, zodSchema } from "../src/index";

const mockSchema = zodSchema({
  parse: (input: unknown) => input as { value: string },
  toJSON: () => ({ type: "object" }),
});

describe("job run handler", () => {
  test("run handler is stored and accessible on returned definition", () => {
    const handler = async (payload: { value: string }) => ({
      processed: payload.value,
    });

    const job = defineJob({
      name: "Handler Job",
      slug: "handler-job",
      endpointUrl: "https://example.com",
      schema: mockSchema,
      run: handler,
    });

    expect(job.run).toBe(handler);
  });

  test("run handler is undefined when not provided", () => {
    const job = defineJob({
      name: "No Handler",
      slug: "no-handler",
      endpointUrl: "https://example.com",
      schema: mockSchema,
    });

    expect(job.run).toBeUndefined();
  });

  test("onSuccess hook is stored on definition", () => {
    // biome-ignore lint/suspicious/noEmptyBlockStatements: noop stub
    const onSuccess = async () => {};
    const job = defineJob({
      name: "Success Hook",
      slug: "success-hook",
      endpointUrl: "https://example.com",
      schema: mockSchema,
      onSuccess,
    });

    expect(job.onSuccess).toBe(onSuccess);
  });

  test("onFailure hook is stored on definition", () => {
    // biome-ignore lint/suspicious/noEmptyBlockStatements: noop stub
    const onFailure = async () => {};
    const job = defineJob({
      name: "Failure Hook",
      slug: "failure-hook",
      endpointUrl: "https://example.com",
      schema: mockSchema,
      onFailure,
    });

    expect(job.onFailure).toBe(onFailure);
  });

  test("onStart hook is stored on definition", () => {
    // biome-ignore lint/suspicious/noEmptyBlockStatements: noop stub
    const onStart = async () => {};
    const job = defineJob({
      name: "Start Hook",
      slug: "start-hook",
      endpointUrl: "https://example.com",
      schema: mockSchema,
      onStart,
    });

    expect(job.onStart).toBe(onStart);
  });

  test("run handler can be invoked directly for testing", async () => {
    const job = defineJob({
      name: "Testable Job",
      slug: "testable-job",
      endpointUrl: "https://example.com",
      schema: mockSchema,
      run: async (payload) => ({ upper: payload.value.toUpperCase() }),
    });

    const result = await job.run?.(
      { value: "hello" },
      {
        runId: "run_test",
        attempt: 1,
        signal: AbortSignal.timeout(5000),
        logger: {
          info: () => undefined,
          warn: () => undefined,
          error: () => undefined,
        },
        checkpoint: () => Promise.resolve(),
        reportProgress: () => Promise.resolve(),
        heartbeat: () => Promise.resolve(),
      }
    );

    expect(result).toEqual({ upper: "HELLO" });
  });

  test("triggerAndWait polls until terminal status", async () => {
    const statuses = ["queued", "executing", "completed"];
    let pollIndex = 0;

    const job = defineJob({
      name: "Wait Job",
      slug: "wait-job",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
    });

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    const result = await job.triggerAndWait(
      {
        createJob: () => Promise.resolve({ id: "job_1" }),
        triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
        getRun: () => {
          const status = statuses[pollIndex] ?? "completed";
          pollIndex += 1;
          return Promise.resolve({ id: "run_1", status });
        },
      },
      { payload: { value: "test" } },
      { initialDelayMs: 1, maxDelayMs: 2, timeoutMs: 100 }
    );

    expect(result.status).toBe("completed");
  });

  test("triggerAndWait throws when client has no getRun", async () => {
    const job = defineJob({
      name: "No GetRun",
      slug: "no-getrun",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
    });

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    await expect(
      job.triggerAndWait(
        {
          createJob: () => Promise.resolve({ id: "job_1" }),
          triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
        },
        { payload: { value: "test" } }
      )
    ).rejects.toThrow("triggerAndWait requires a client with getRun method");
  });
});
