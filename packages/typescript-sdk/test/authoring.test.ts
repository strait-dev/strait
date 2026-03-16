import { describe, expect, test } from "bun:test";
import { Schema } from "effect";

import {
  defineDag,
  defineJob,
  defineWorkflow,
  effectSchema,
  isErr,
  zodSchema,
} from "../src/index";

describe("authoring DSL", () => {
  test("defineJob registers and triggers using validated payloads", async () => {
    let registrationBody: Record<string, unknown> | undefined;
    let triggerBody: Record<string, unknown> | undefined;

    const job = defineJob({
      name: "Sync Inventory",
      slug: "sync-inventory",
      endpointUrl: "https://worker.dev/jobs/sync",
      projectId: "proj_1",
      schema: zodSchema({
        parse: (input: unknown) => {
          if (typeof input !== "object" || input === null) {
            throw new Error("invalid payload");
          }

          return input as { sku: string };
        },
        toJSON: () => ({ type: "object" }),
      }),
    });

    const createResponse = await job.register(
      {
        createJob: (input: { readonly body: unknown }) => {
          registrationBody = input.body as Record<string, unknown>;
          return Promise.resolve({ id: "job_1" });
        },
        triggerJob: () => Promise.resolve({ id: "run_1" }),
      },
      {}
    );

    expect(createResponse).toEqual({ id: "job_1" });
    expect(registrationBody?.project_id).toBe("proj_1");
    expect(registrationBody?.payload_schema).toEqual({ type: "object" });

    const triggerResponse = await job.trigger(
      {
        createJob: () => Promise.resolve({ id: "job_1" }),
        triggerJob: (input: { readonly body?: unknown }) => {
          triggerBody = input.body as Record<string, unknown>;
          return Promise.resolve({ id: "run_1" });
        },
      },
      {
        payload: { sku: "abc" },
      }
    );

    expect(triggerResponse).toEqual({ id: "run_1" });
    expect(triggerBody?.payload).toEqual({ sku: "abc" });
  });

  test("defineJob triggerResult returns Result wrapper", async () => {
    const job = defineJob({
      name: "Failing",
      slug: "failing-job",
      endpointUrl: "https://worker.dev/jobs/failing",
      projectId: "proj_1",
      schema: effectSchema(Schema.Struct({ value: Schema.String })),
    });

    await job.register(
      {
        createJob: () => Promise.resolve({ id: "job_2" }),
        triggerJob: () => Promise.resolve({ id: "run_2" }),
      },
      {}
    );

    const result = await job.triggerResult(
      {
        createJob: () => Promise.resolve({ id: "job_2" }),
        triggerJob: () => Promise.reject(new Error("boom")),
      },
      {
        payload: { value: "x" },
      }
    );

    expect(isErr(result)).toBe(true);
  });

  test("defineWorkflow and defineDag expose register-only authoring flow", async () => {
    let workflowRegistered = false;
    let dagHookKind = "";

    const workflow = defineWorkflow({
      name: "Order pipeline",
      slug: "order-pipeline",
      projectId: "proj_1",
      steps: [{ step_ref: "step-1", job_id: "job_1" }],
      schema: effectSchema(Schema.Struct({ order_id: Schema.String })),
    });

    await workflow.register(
      {
        createWorkflow: () => {
          workflowRegistered = true;
          return Promise.resolve({ id: "wf_1" });
        },
        triggerWorkflow: () => Promise.resolve({ id: "wfr_1" }),
      },
      {}
    );

    expect(workflowRegistered).toBe(true);

    const dag = defineDag({
      name: "DAG",
      slug: "dag-flow",
      projectId: "proj_1",
      steps: [{ step_ref: "step-1", job_id: "job_1" }],
      schema: effectSchema(Schema.Struct({ run_id: Schema.String })),
      hooks: {
        onRegister: (context: { readonly kind: string }) => {
          dagHookKind = context.kind;
        },
      },
    });

    await dag.register(
      {
        createWorkflow: () => Promise.resolve({ id: "wf_2" }),
        triggerWorkflow: () => Promise.resolve({ id: "wfr_2" }),
      },
      {}
    );

    expect(dag.kind).toBe("dag");
    expect(dagHookKind).toBe("dag");
  });
});
