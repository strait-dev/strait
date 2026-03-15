import { describe, expect, test } from "bun:test";
import { Schema } from "effect";

import { defineWorkflow, effectSchema } from "../src/index";

const mockSchema = effectSchema(Schema.Struct({ orderId: Schema.String }));

describe("DefineWorkflowOptions → toRegistrationBody mapping", () => {
  test("maps all basic fields to snake_case", () => {
    const wf = defineWorkflow({
      name: "Order Pipeline",
      slug: "order-pipeline",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [{ step_ref: "step-1", job_id: "job_1" }],
      description: "Process orders",
      tags: { team: "orders" },
      environmentId: "env_prod",
    });

    const body = wf.toRegistrationBody();
    expect(body.project_id).toBe("proj_1");
    expect(body.name).toBe("Order Pipeline");
    expect(body.slug).toBe("order-pipeline");
    expect(body.description).toBe("Process orders");
    expect(body.tags).toEqual({ team: "orders" });
    expect(body.environment_id).toBe("env_prod");
    expect(body.steps).toEqual([{ step_ref: "step-1", job_id: "job_1" }]);
  });

  test("maps concurrency and parallelism fields", () => {
    const wf = defineWorkflow({
      name: "Parallel WF",
      slug: "parallel-wf",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [],
      maxConcurrentRuns: 10,
      maxParallelSteps: 3,
    });

    const body = wf.toRegistrationBody();
    expect(body.max_concurrent_runs).toBe(10);
    expect(body.max_parallel_steps).toBe(3);
  });

  test("maps timeout, retry, cron fields", () => {
    const wf = defineWorkflow({
      name: "Full WF",
      slug: "full-wf",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [],
      timeoutSecs: 600,
      maxAttempts: 3,
      retryStrategy: "exponential",
      cron: "0 * * * *",
      timezone: "UTC",
    });

    const body = wf.toRegistrationBody();
    expect(body.timeout_secs).toBe(600);
    expect(body.max_attempts).toBe(3);
    expect(body.retry_strategy).toBe("exponential");
    expect(body.cron).toBe("0 * * * *");
    expect(body.timezone).toBe("UTC");
  });

  test("maps webhook fields", () => {
    const wf = defineWorkflow({
      name: "Webhook WF",
      slug: "webhook-wf",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [],
      webhookUrl: "https://hooks.example.com",
      webhookSecret: "whsec_456",
    });

    const body = wf.toRegistrationBody();
    expect(body.webhook_url).toBe("https://hooks.example.com");
    expect(body.webhook_secret).toBe("whsec_456");
  });

  test("typed trigger sends all fields as snake_case", async () => {
    let capturedBody: Record<string, unknown> = {};

    const wf = defineWorkflow({
      name: "Trigger WF",
      slug: "trigger-wf",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [],
    });

    await wf.register({
      createWorkflow: () => Promise.resolve({ id: "wf_1" }),
      triggerWorkflow: (input) => {
        capturedBody = (input.body ?? {}) as Record<string, unknown>;
        return Promise.resolve({ id: "wfr_1", status: "pending" });
      },
    });

    await wf.trigger(
      {
        createWorkflow: () => Promise.resolve({ id: "wf_1" }),
        triggerWorkflow: (input) => {
          capturedBody = (input.body ?? {}) as Record<string, unknown>;
          return Promise.resolve({ id: "wfr_1", status: "pending" });
        },
      },
      {
        payload: { orderId: "ord_123" },
        idempotencyKey: "idem_1",
        priority: 5,
        dryRun: true,
        metadata: { source: "test" },
        stepOverrides: { step1: { timeout: 30 } },
      }
    );

    expect(capturedBody.idempotency_key).toBe("idem_1");
    expect(capturedBody.priority).toBe(5);
    expect(capturedBody.dry_run).toBe(true);
    expect(capturedBody.metadata).toEqual({ source: "test" });
    expect(capturedBody.step_overrides).toEqual({ step1: { timeout: 30 } });
  });

  test("workflow returns typed WorkflowRunResponse from trigger", async () => {
    const wf = defineWorkflow({
      name: "Typed WF",
      slug: "typed-wf",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [],
    });

    await wf.register({
      createWorkflow: () => Promise.resolve({ id: "wf_1" }),
      triggerWorkflow: () =>
        Promise.resolve({ id: "wfr_1", status: "pending" }),
    });

    const run = await wf.trigger(
      {
        createWorkflow: () => Promise.resolve({ id: "wf_1" }),
        triggerWorkflow: () =>
          Promise.resolve({ id: "wfr_1", status: "pending" }),
      },
      { payload: { orderId: "ord_1" } }
    );

    expect(run.id).toBe("wfr_1");
    expect(run.status).toBe("pending");
  });
});
