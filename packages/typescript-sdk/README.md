# @strait/ts

Effect-first TypeScript SDK for Strait with generated endpoint coverage, high-level Promise helpers, and authoring primitives for jobs/workflows.

## Status

In active implementation.

## What this package provides

- Generated low-level operation/domain clients from `docs/openapi.yaml`
- Promise-first high-level API (`client.createJob(...)`, `client.jobs.create(...)`)
- Result variants for non-GET operations (`client.createJobResult(...)`)
- Composition helpers for retries, idempotency headers, run polling, pagination, and deployment lifecycle orchestration
- Authoring DSL with full `defineJob`/`defineWorkflow`/`defineDag` configuration, `run` handler, lifecycle hooks, and typed step builder
- Schema adapter bridge helpers for Effect and Zod-like schemas
- XState FSMs modeling job run, workflow run, and step run state machines
- DAG validation with Kahn's algorithm for workflow steps
- AbortSignal support across HTTP client, retries, and polling
- Request/response middleware hooks

## Client creation

### Inline config

```ts
import { createClient } from "@strait/ts";

const client = createClient({
  baseUrl: "http://localhost:3000",
  auth: { type: "bearer", token: process.env.STRAIT_API_KEY! },
  timeoutMs: 30_000,
}, {
  middleware: [{
    onRequest: ({ method, url }) => console.log(`→ ${method} ${url}`),
    onResponse: ({ status, durationMs }) => console.log(`← ${status} (${durationMs}ms)`),
  }],
});
```

### Config file discovery (`strait.config.ts`)
> Node/Bun environments only. Import from `@strait/ts/node`.

```ts
// strait.config.ts
import { defineStraitConfig } from "@strait/ts/node";

export default defineStraitConfig({
  baseUrl: "http://localhost:3000",
  auth: { type: "bearer", token: process.env.STRAIT_API_KEY! },
});
```

```ts
// app.ts
import { createClientFromConfigFile } from "@strait/ts/node";

const client = await createClientFromConfigFile();
```

## Calling generated operations

### High-level top-level methods

```ts
const created = await client.createJob({
  body: {
    project_id: "proj_1",
    name: "Sync inventory",
    slug: "sync-inventory",
    endpoint_url: "https://worker.example/jobs/sync",
  },
});
```

### Namespaced methods

```ts
const list = await client.jobs.list({
  query: { project_id: "proj_1" },
});
```

### Result variants for non-GET operations

```ts
const result = await client.createJobResult({
  body: {
    project_id: "proj_1",
    name: "Sync inventory",
    slug: "sync-inventory",
    endpoint_url: "https://worker.example/jobs/sync",
  },
});

if (!result.ok) {
  console.error(result.error);
}
```

## Authoring DSL

### Full-featured job definition with `run` handler

```ts
import { createClient, defineJob, zodSchema } from "@strait/ts";
import { z } from "zod";

const syncInventory = defineJob({
  name: "Sync Inventory",
  slug: "sync-inventory",
  endpointUrl: "https://worker.dev/jobs/sync",
  projectId: "proj_1",
  schema: zodSchema(z.object({ sku: z.string() })),

  // Scheduling
  cron: "*/5 * * * *",
  timezone: "America/New_York",

  // Concurrency & rate limiting
  maxConcurrency: 5,
  rateLimitMax: 100,
  rateLimitWindowSecs: 60,

  // Retry
  maxAttempts: 5,
  retryStrategy: "exponential",
  timeoutSecs: 300,

  // Tags
  tags: { team: "inventory" },

  // Run handler
  run: async (payload, ctx) => {
    ctx.logger.info("Starting sync", { sku: payload.sku });
    await ctx.reportProgress(0.1);

    const result = await fetchInventory(payload.sku);
    await ctx.checkpoint({ fetched: true });
    await ctx.reportProgress(1.0);

    return { synced: true, count: result.items.length };
  },

  // Lifecycle hooks
  onSuccess: async ({ output }) => console.log("Synced", output.count, "items"),
  onFailure: async ({ error }) => alertOncall(error),
});

// Register, trigger, wait
const job = await syncInventory.register(client);
const run = await syncInventory.trigger(client, {
  payload: { sku: "ABC-123" },
  priority: 10,
  idempotencyKey: "sync-abc-123",
});

// Trigger and wait for completion
const completed = await syncInventory.triggerAndWait(client, {
  payload: { sku: "ABC-123" },
}, { timeoutMs: 120_000 });

// Batch trigger
await syncInventory.batchTrigger(client, {
  items: [
    { payload: { sku: "A" } },
    { payload: { sku: "B" }, priority: 5 },
  ],
});
```

### Workflow with typed step builder

```ts
import { defineWorkflow, step, validateDag } from "@strait/ts";

const orderPipeline = defineWorkflow({
  name: "Order Pipeline",
  slug: "order-pipeline",
  projectId: "proj_1",
  schema: zodSchema(z.object({ orderId: z.string() })),
  maxConcurrentRuns: 10,
  maxParallelSteps: 3,
  steps: [
    step.job("validate", "job_validate"),
    step.job("charge", "job_charge", {
      dependsOn: ["validate"],
      onFailure: "fail_workflow",
      retryMaxAttempts: 3,
      retryBackoff: "exponential",
    }),
    step.approval("review", {
      dependsOn: ["charge"],
      approvalTimeoutSecs: 3600,
      approvers: ["admin@example.com"],
    }),
    step.waitForEvent("shipping", "shipping.confirmed", {
      dependsOn: ["review"],
      eventTimeoutSecs: 86400,
    }),
    step.sleep("cooldown", 60, { dependsOn: ["shipping"] }),
    step.subWorkflow("notify-all", "wf_notifications", {
      dependsOn: ["cooldown"],
      maxNestingDepth: 2,
    }),
  ],
});

// DAG is validated at definition time
```

### DAG validation

```ts
import { step, validateDag } from "@strait/ts";

const sorted = validateDag([
  step.job("a", "job_1"),
  step.job("b", "job_2", { dependsOn: ["a"] }),
  step.job("c", "job_3", { dependsOn: ["a", "b"] }),
]);
// sorted = ["a", "b", "c"]

// Circular dependencies throw DagValidationError
validateDag([
  step.job("a", "j1", { dependsOn: ["b"] }),
  step.job("b", "j2", { dependsOn: ["a"] }),
]); // throws DagValidationError
```

## Composition helpers

```ts
import {
  collectAll,
  fromPromise,
  paginate,
  triggerAndWait,
  waitForRun,
  withIdempotency,
  withRetry,
} from "@strait/ts";

// Retry with jitter
const run = await withRetry(
  () => client.triggerJob({ pathParams: { jobID: "job_1" }, body: { payload: { sku: "sku_1" } } }),
  { attempts: 5, delayMs: 250, jitter: "full" }
);

// Wait for run
await waitForRun(client.getRun, run.id, { timeoutMs: 120_000 });

// Standalone trigger + wait
const result = await triggerAndWait(
  (input) => client.triggerJob({ pathParams: { jobID: "job_1" }, body: input }),
  (input) => client.getRun(input),
  { payload: { sku: "ABC-123" } },
  { timeoutMs: 120_000 },
);

// Paginate through results
for await (const r of paginate((q) => client.listRuns({ query: q }))) {
  console.log(r.id, r.status);
}

// Or collect all at once
const allRuns = await collectAll(paginate((q) => client.listRuns({ query: q })));
```

### Deployment lifecycle helpers

```ts
import {
  createFinalizePromoteDeployment,
  rollbackDeploymentVersion,
} from "@strait/ts";

const promoted = await createFinalizePromoteDeployment(client, {
  create: {
    body: {
      project_id: "proj_1",
      environment: "staging",
      runtime: "node",
      artifact_uri: "file:///tmp/manifest.json",
    },
  },
});

await rollbackDeploymentVersion(client, {
  deploymentID: promoted.promoted.id!,
  body: {
    project_id: "proj_1",
    environment: "staging",
  },
});
```

## FSM (State Machines)

XState v5 state machines modeling run lifecycles for client-side validation and UI state management.

```ts
import { createActor } from "xstate";
import {
  canTransitionRun,
  isTerminalRunStatus,
  runMachine,
  workflowRunMachine,
  stepRunMachine,
} from "@strait/ts";

// Validate transitions
canTransitionRun("executing", "COMPLETE"); // true
canTransitionRun("completed", "EXECUTE"); // false

// Check terminal status
isTerminalRunStatus("completed"); // true
isTerminalRunStatus("executing"); // false

// Create an actor for advanced use cases
const actor = createActor(runMachine);
actor.start();
actor.send({ type: "ENQUEUE" });
actor.send({ type: "DEQUEUE" });
actor.send({ type: "EXECUTE" });
actor.getSnapshot().value; // "executing"
```

## Quality checks

```bash
cd packages/typescript-sdk && bun run run-all
cd packages/typescript-sdk && bun test
```
