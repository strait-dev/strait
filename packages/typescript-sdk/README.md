# @strait/ts

Effect-first TypeScript SDK for Strait with generated endpoint coverage, high-level Promise helpers, and authoring primitives for jobs/workflows.

## Status

In active implementation.

## What this package provides

- Generated low-level operation/domain clients from `docs/openapi.yaml`
- Promise-first high-level API (`client.createJob(...)`, `client.jobs.create(...)`)
- Result variants for non-GET operations (`client.createJobResult(...)`)
- Composition helpers for retries, idempotency headers, and run polling
- Authoring DSL helpers for defining jobs/workflows/DAGs
- Schema adapter bridge helpers for Effect and Zod-like schemas

## Client creation

### Inline config

```ts
import { createClient } from "@strait/ts";

const client = createClient({
  baseUrl: "http://localhost:3000",
  auth: { type: "bearer", token: process.env.STRAIT_API_KEY! },
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
  // typed error channel without throwing
  console.error(result.error);
}
```

## Composition helpers

```ts
import {
  fromPromise,
  waitForRun,
  withIdempotency,
  withRetry,
} from "@strait/ts";

const triggerInput = withIdempotency(
  { body: { payload: { sku: "sku_1" } } },
  crypto.randomUUID()
);

const run = await withRetry(
  () => client.triggerJob(triggerInput),
  { attempts: 5, delayMs: 250 }
);

await waitForRun(client.getRun, run.id, { timeoutMs: 120_000 });

const safeResult = await fromPromise(() => client.deleteJob({
  pathParams: { jobID: "job_123" },
}));
```

## Authoring DSL

```ts
import { defineJob, effectSchema, zodSchema } from "@strait/ts";
import { Schema } from "effect";

const syncJob = defineJob({
  name: "Sync inventory",
  slug: "sync-inventory",
  endpointUrl: "https://worker.example/jobs/sync",
  projectId: "proj_1",
  schema: effectSchema(Schema.Struct({ sku: Schema.String })),
});

await syncJob.register(client, {});
await syncJob.trigger(client, { payload: { sku: "sku_1" } });

const zodJob = defineJob({
  name: "Sync inventory (zod)",
  slug: "sync-inventory-zod",
  endpointUrl: "https://worker.example/jobs/sync",
  projectId: "proj_1",
  schema: zodSchema(myZodSchema),
});
```

Related helpers: `defineWorkflow(...)`, `defineDag(...)`, `effectSchema(...)`, `zodSchema(...)`.

## Quality checks

```bash
cd packages/typescript-sdk && bun run run-all
cd packages/typescript-sdk && bun test
```