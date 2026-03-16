import { describe, expect, test } from "bun:test";
import { Schema } from "effect";
import { createClient } from "../src/client";
import { DagValidationError } from "../src/errors";
import { defineDag, defineWorkflow, effectSchema, step } from "../src/index";
import type { FetchLike } from "../src/runtime";

const mockSchema = effectSchema(Schema.Struct({ orderId: Schema.String }));

describe("stepToApi wiring — typed steps are converted to snake_case on registration", () => {
  test("step builder fields become snake_case in registration body", () => {
    const wf = defineWorkflow({
      name: "Pipeline",
      slug: "pipeline",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [
        step.job("validate", "job_validate"),
        step.job("charge", "job_charge", {
          dependsOn: ["validate"],
          onFailure: "fail_workflow",
          retryMaxAttempts: 3,
        }),
      ],
    });

    const body = wf.toRegistrationBody();
    const steps = body.steps as Record<string, unknown>[];

    // Should be snake_case, not camelCase
    expect(steps[0]?.step_ref).toBe("validate");
    expect(steps[0]?.type).toBe("job");
    expect(steps[0]?.job_id).toBe("job_validate");

    expect(steps[1]?.step_ref).toBe("charge");
    expect(steps[1]?.depends_on).toEqual(["validate"]);
    expect(steps[1]?.on_failure).toBe("fail_workflow");
    expect(steps[1]?.retry_max_attempts).toBe(3);

    // camelCase fields should NOT be present
    expect(steps[1]?.stepRef).toBeUndefined();
    expect(steps[1]?.dependsOn).toBeUndefined();
    expect(steps[1]?.onFailure).toBeUndefined();
    expect(steps[1]?.retryMaxAttempts).toBeUndefined();
  });

  test("all 5 step types are converted correctly", () => {
    const wf = defineWorkflow({
      name: "Full",
      slug: "full",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [
        step.job("a", "job_a"),
        step.approval("b", { dependsOn: ["a"], approvalTimeoutSecs: 3600 }),
        step.subWorkflow("c", "wf_child", {
          dependsOn: ["b"],
          maxNestingDepth: 2,
        }),
        step.waitForEvent("d", "event.key", {
          dependsOn: ["c"],
          eventTimeoutSecs: 600,
        }),
        step.sleep("e", 30, { dependsOn: ["d"] }),
      ],
    });

    const body = wf.toRegistrationBody();
    const steps = body.steps as Record<string, unknown>[];

    expect(steps[1]?.approval_timeout_secs).toBe(3600);
    expect(steps[2]?.sub_workflow_id).toBe("wf_child");
    expect(steps[2]?.max_nesting_depth).toBe(2);
    expect(steps[3]?.event_key).toBe("event.key");
    expect(steps[3]?.event_timeout_secs).toBe(600);
    expect(steps[4]?.sleep_duration_secs).toBe(30);
  });

  test("raw object steps are passed through unchanged", () => {
    const wf = defineWorkflow({
      name: "Raw",
      slug: "raw",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [{ step_ref: "legacy", job_id: "job_old", type_raw: "custom" }],
    });

    const body = wf.toRegistrationBody();
    const steps = body.steps as Record<string, unknown>[];

    expect(steps[0]?.step_ref).toBe("legacy");
    expect(steps[0]?.job_id).toBe("job_old");
  });

  test("defineDag also converts steps", () => {
    const dag = defineDag({
      name: "DAG",
      slug: "dag",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [
        step.job("x", "job_x"),
        step.job("y", "job_y", { dependsOn: ["x"] }),
      ],
    });

    const body = dag.toRegistrationBody();
    const steps = body.steps as Record<string, unknown>[];

    expect(steps[0]?.step_ref).toBe("x");
    expect(steps[0]?.job_id).toBe("job_x");
    expect(steps[1]?.depends_on).toEqual(["x"]);
  });
});

describe("validateDag wiring — invalid DAGs throw at registration time", () => {
  test("circular dependency throws DagValidationError on toRegistrationBody", () => {
    const wf = defineWorkflow({
      name: "Cycle",
      slug: "cycle",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [
        step.job("a", "job_a", { dependsOn: ["b"] }),
        step.job("b", "job_b", { dependsOn: ["a"] }),
      ],
    });

    expect(() => wf.toRegistrationBody()).toThrow(DagValidationError);
  });

  test("missing ref throws DagValidationError on toRegistrationBody", () => {
    const wf = defineWorkflow({
      name: "Missing",
      slug: "missing",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [
        step.job("a", "job_a"),
        step.job("b", "job_b", { dependsOn: ["nonexistent"] }),
      ],
    });

    expect(() => wf.toRegistrationBody()).toThrow(DagValidationError);
  });

  test("duplicate refs throw DagValidationError on toRegistrationBody", () => {
    const wf = defineWorkflow({
      name: "Dups",
      slug: "dups",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [step.job("a", "job_a"), step.job("a", "job_b")],
    });

    expect(() => wf.toRegistrationBody()).toThrow(DagValidationError);
  });

  test("circular dependency also throws for defineDag", () => {
    const dag = defineDag({
      name: "Cycle DAG",
      slug: "cycle-dag",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [
        step.job("a", "job_a", { dependsOn: ["b"] }),
        step.job("b", "job_b", { dependsOn: ["a"] }),
      ],
    });

    expect(() => dag.toRegistrationBody()).toThrow(DagValidationError);
  });

  test("valid DAG does not throw", () => {
    const wf = defineWorkflow({
      name: "Valid",
      slug: "valid",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [
        step.job("a", "job_a"),
        step.job("b", "job_b", { dependsOn: ["a"] }),
        step.job("c", "job_c", { dependsOn: ["a", "b"] }),
      ],
    });

    expect(() => wf.toRegistrationBody()).not.toThrow();
  });

  test("raw object steps skip DAG validation", () => {
    const wf = defineWorkflow({
      name: "Legacy",
      slug: "legacy",
      projectId: "proj_1",
      schema: mockSchema,
      steps: [{ step_ref: "x", job_id: "job_x" }],
    });

    // Should not throw — raw steps have no stepRef/type so are skipped
    expect(() => wf.toRegistrationBody()).not.toThrow();
  });
});

describe("middleware wiring — hooks fire on actual HTTP requests", () => {
  test("onRequest fires with method and url", async () => {
    const calls: { method: string; url: string }[] = [];

    const fetchImpl: FetchLike = () =>
      Promise.resolve(
        new Response(JSON.stringify({ ok: true }), { status: 200 })
      );

    const client = createClient(
      {
        baseUrl: "https://api.test.io",
        auth: { type: "bearer", token: "tok_123" },
      },
      {
        fetch: fetchImpl,
        middleware: [
          {
            onRequest: (ctx) => {
              calls.push({ method: ctx.method, url: ctx.url });
            },
          },
        ],
      }
    );

    await client.operationsPromise.getHealth();

    expect(calls.length).toBe(1);
    expect(calls[0]?.method).toBe("GET");
    expect(calls[0]?.url).toContain("/health");
  });

  test("onResponse fires with status and durationMs", async () => {
    const responses: { status: number; durationMs: number }[] = [];

    const fetchImpl: FetchLike = () =>
      Promise.resolve(
        new Response(JSON.stringify({ status: "ok" }), { status: 200 })
      );

    const client = createClient(
      {
        baseUrl: "https://api.test.io",
        auth: { type: "bearer", token: "tok_123" },
      },
      {
        fetch: fetchImpl,
        middleware: [
          {
            onResponse: (ctx) => {
              responses.push({
                status: ctx.status,
                durationMs: ctx.durationMs,
              });
            },
          },
        ],
      }
    );

    await client.operationsPromise.getHealth();

    expect(responses.length).toBe(1);
    expect(responses[0]?.status).toBe(200);
    expect(typeof responses[0]?.durationMs).toBe("number");
  });

  test("onError fires when fetch throws", async () => {
    const errors: { method: string; error: unknown }[] = [];

    const fetchImpl: FetchLike = () =>
      Promise.reject(new Error("network down"));

    const client = createClient(
      {
        baseUrl: "https://api.test.io",
        auth: { type: "bearer", token: "tok_123" },
      },
      {
        fetch: fetchImpl,
        middleware: [
          {
            onError: (ctx) => {
              errors.push({ method: ctx.method, error: ctx.error });
            },
          },
        ],
      }
    );

    try {
      await client.operationsPromise.getHealth();
    } catch {
      // Expected to throw TransportError
    }

    expect(errors.length).toBe(1);
    expect(errors[0]?.method).toBe("GET");
    expect(errors[0]?.error).toBeInstanceOf(Error);
  });

  test("multiple middleware execute in order", async () => {
    const order: string[] = [];

    const fetchImpl: FetchLike = () =>
      Promise.resolve(
        new Response(JSON.stringify({ ok: true }), { status: 200 })
      );

    const client = createClient(
      {
        baseUrl: "https://api.test.io",
        auth: { type: "bearer", token: "tok_123" },
      },
      {
        fetch: fetchImpl,
        middleware: [
          {
            onRequest: () => {
              order.push("first");
            },
          },
          {
            onRequest: () => {
              order.push("second");
            },
          },
        ],
      }
    );

    await client.operationsPromise.getHealth();

    expect(order).toEqual(["first", "second"]);
  });
});
