import { describe, expect, test } from "bun:test";

import { defineJob, zodSchema } from "../src/index";

const mockSchema = zodSchema({
  parse: (input: unknown) => input as { sku: string },
  toJSON: () => ({ type: "object" }),
});

describe("typed trigger options", () => {
  test("trigger sends typed fields as snake_case", async () => {
    let capturedBody: Record<string, unknown> = {};

    const job = defineJob({
      name: "Trigger Test",
      slug: "trigger-test",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
    });

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    await job.trigger(
      {
        createJob: () => Promise.resolve({ id: "job_1" }),
        triggerJob: (input) => {
          capturedBody = (input.body ?? {}) as Record<string, unknown>;
          return Promise.resolve({ id: "run_1", status: "queued" });
        },
      },
      {
        payload: { sku: "ABC-123" },
        idempotencyKey: "idem_abc",
        priority: 10,
        dryRun: true,
        metadata: { source: "test" },
        scheduledAt: "2026-03-15T10:00:00Z",
      }
    );

    expect(capturedBody.payload).toEqual({ sku: "ABC-123" });
    expect(capturedBody.idempotency_key).toBe("idem_abc");
    expect(capturedBody.priority).toBe(10);
    expect(capturedBody.dry_run).toBe(true);
    expect(capturedBody.metadata).toEqual({ source: "test" });
    expect(capturedBody.scheduled_at).toBe("2026-03-15T10:00:00Z");
  });

  test("trigger returns typed JobRunResponse", async () => {
    const job = defineJob({
      name: "Typed Trigger",
      slug: "typed-trigger",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
    });

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    const run = await job.trigger(
      {
        createJob: () => Promise.resolve({ id: "job_1" }),
        triggerJob: () =>
          Promise.resolve({ id: "run_1", status: "queued" }),
      },
      { payload: { sku: "test" } }
    );

    expect(run.id).toBe("run_1");
    expect(run.status).toBe("queued");
  });

  test("register returns typed JobResponse", async () => {
    const job = defineJob({
      name: "Typed Register",
      slug: "typed-register",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
    });

    const response = await job.register({
      createJob: () =>
        Promise.resolve({ id: "job_1", slug: "typed-register", name: "Typed Register" }),
      triggerJob: () => Promise.resolve({ id: "run_1" }),
    });

    expect(response.id).toBe("job_1");
    expect(response.slug).toBe("typed-register");
    expect(response.name).toBe("Typed Register");
  });

  test("batchTrigger sends items in correct format", async () => {
    let capturedBody: Record<string, unknown> = {};

    const job = defineJob({
      name: "Batch Job",
      slug: "batch-job",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
    });

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    await job.batchTrigger(
      {
        createJob: () => Promise.resolve({ id: "job_1" }),
        triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
        triggerJobBulk: (input) => {
          capturedBody = (input.body ?? {}) as Record<string, unknown>;
          return Promise.resolve({
            runs: [{ id: "run_1" }, { id: "run_2" }],
          });
        },
      },
      {
        items: [
          { payload: { sku: "A" } },
          { payload: { sku: "B" }, priority: 5, idempotencyKey: "idem_b" },
        ],
      }
    );

    const items = capturedBody.items as Record<string, unknown>[];
    expect(items.length).toBe(2);
    expect(items[0]!.payload).toEqual({ sku: "A" });
    expect(items[1]!.payload).toEqual({ sku: "B" });
    expect(items[1]!.priority).toBe(5);
    expect(items[1]!.idempotency_key).toBe("idem_b");
  });

  test("batchTrigger throws when client lacks triggerJobBulk", async () => {
    const job = defineJob({
      name: "No Bulk",
      slug: "no-bulk",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
    });

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    await expect(
      job.batchTrigger(
        {
          createJob: () => Promise.resolve({ id: "job_1" }),
          triggerJob: () =>
            Promise.resolve({ id: "run_1", status: "queued" }),
        },
        { items: [{ payload: { sku: "A" } }] }
      )
    ).rejects.toThrow("triggerJobBulk");
  });

  test("trigger omits undefined optional fields", async () => {
    let capturedBody: Record<string, unknown> = {};

    const job = defineJob({
      name: "Minimal Trigger",
      slug: "minimal-trigger",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
    });

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    await job.trigger(
      {
        createJob: () => Promise.resolve({ id: "job_1" }),
        triggerJob: (input) => {
          capturedBody = (input.body ?? {}) as Record<string, unknown>;
          return Promise.resolve({ id: "run_1", status: "queued" });
        },
      },
      { payload: { sku: "test" } }
    );

    expect(capturedBody.payload).toEqual({ sku: "test" });
    expect(capturedBody.idempotency_key).toBeUndefined();
    expect(capturedBody.priority).toBeUndefined();
    expect(capturedBody.dry_run).toBeUndefined();
  });
});
