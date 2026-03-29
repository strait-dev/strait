# `@strait/agents`

`packages/agents-sdk` is the public runtime-facing TypeScript SDK used by managed Strait agents while they are executing.

This package is not the public management SDK. It is the in-run callback layer that talks to `/sdk/v1/runs/{runID}/...`.

It exists to make agent runtimes easy to implement consistently across:

- local execution
- Cloudflare Workers
- provider adapter wrappers
- workflow-oriented orchestration helpers

## What This Package Provides

### Core Context

- `StraitContext`
- `createStraitContext()`

The context handles:

- authentication with the run token
- callback retries
- logs and progress
- checkpoints
- usage reporting
- tool-call reporting
- stream chunk reporting
- completion and failure
- run-scoped state
- workflow-scoped state

### Agent Definitions

- `strait.agent()`
- `agent`

These define managed agent handlers with normalized budgets and runtime metadata.

### AI Step Helpers

- `createAIStep()`

These helpers wrap local inference and tool loops with checkpointed, telemetry-aware behavior.

### Provider Adapters

- `@strait/agents/ai-sdk`
- `@strait/agents/openai`
- `@strait/agents/anthropic`

These wrappers preserve the provider client surface while layering in:

- budget checks
- usage capture
- stream forwarding
- tool-call telemetry
- checkpoint integration

### Workflow Helpers

- `agentStep()`
- `approvalStep()`
- `agentWorkflow()`
- `createDynamicSteps()`
- `fanOutSteps()`
- `pipelinePattern()`
- `debatePattern()`
- `orchestratorPattern()`
- `waitForEventStep()`
- `sleepStep()`
- `subWorkflowStep()`

These helpers let agents participate in Strait workflows without inventing a second workflow engine.

### Eval Helpers

- `defineEvalSuite()`
- `runEvalSuite()`
- `expectTextContains()`
- `expectPathEquals()`
- `expectArrayMinLength()`

These helpers support local regression checks for agents and orchestration logic.

### Sandbox Metadata

- `createSandboxTool()`

Sandbox tools carry policy metadata used by the local runtime and the Cloudflare Dynamic Workers path, with outbound-worker mode kept as a compatibility fallback.

## Architecture

The SDK sits between the runtime and the Go control plane:

1. the runtime receives a dispatch envelope
2. the runtime creates a `StraitContext`
3. runtime code and adapters report events through the SDK
4. the SDK calls the Go callback endpoints with the run token
5. the Go control plane persists those events into the existing run telemetry tables

This keeps execution runtimes thin and ensures provider wrappers, tools, and workflow helpers all speak the same callback contract.

## Installation

```bash
bun add @strait/agents
```

## Minimal Usage

```ts
import { StraitContext, strait } from "@strait/agents";

export const summarizeAgent = strait.agent({
  name: "Summarize",
  slug: "summarize",
  model: "gpt-5.4-mini",
  async run(ctx: StraitContext, input: { topic: string }) {
    await ctx.log({ level: "info", message: `summarizing ${input.topic}` });
    await ctx.progress({ percent: 20, message: "collecting context" });
    await ctx.checkpoint({ phase: "collecting" });

    return {
      topic: input.topic,
      summary: `Prepared a summary for ${input.topic}.`,
    };
  },
});
```

## Creating Contexts

From environment:

```ts
import { StraitContext } from "@strait/agents";

const ctx = StraitContext.fromEnv();
```

Manual construction:

```ts
import { createStraitContext } from "@strait/agents";

const ctx = createStraitContext({
  baseUrl: "http://127.0.0.1:8080",
  runId: "run_123",
  runToken: "jwt-token",
});
```

## Budgets and Pricing

The SDK includes:

- pricing catalog helpers
- usage normalization
- budget ledgers
- budget guard errors

Budgets can be declared in normalized microusd values or user-friendly strings like `"$5.00"`, which are normalized during agent definition.

## Dynamic Workflows

Planner-style steps can emit validated runtime DAG expansion requests:

```ts
import { createDynamicSteps, fanOutSteps } from "@strait/agents";

const steps = fanOutSteps({
  dependsOn: ["planner"],
  stepRefPrefix: "research",
  synthesizer: { stepRef: "synthesis", agentId: "agent_synthesizer" },
  workers: [{ agentId: "agent_logs" }, { agentId: "agent_metrics" }],
});

const dynamic = createDynamicSteps(steps, { knownStepRefs: ["planner"] });
```

This aligns with the workflow engine’s `dynamic_steps` contract and is validated before runtime expansion.

## Sandbox Tools

Use `createSandboxTool()` when tool execution should carry sandbox metadata:

```ts
import { createSandboxTool } from "@strait/agents";

const fetchTool = createSandboxTool({
  name: "web-fetch",
  timeoutMs: 30_000,
  sandbox: {
    executionMode: "sandboxed",
    networkClass: "restricted",
    outboundPolicyTag: "research",
  },
  execute: async (input: { url: string }) => ({ url: input.url, ok: true }),
});
```

## Examples

Reference examples live in:

- `packages/agents-sdk/examples/incident-triage-agent.ts`
- `packages/agents-sdk/examples/dynamic-planner-workflow.ts`
- `packages/agents-sdk/examples/multi-agent-research-pipeline.ts`
- `packages/agents-sdk/examples/agent-escalates-to-workflow.ts`

## Validation

Run the package checks with:

```bash
cd packages/agents-sdk
bun run test
bun run typecheck
bun run biome:lint
```

## Related References

- `apps/docs/sdks/agents.mdx`
- `apps/docs/concepts/agents.mdx`
- `apps/docs/guides/local-agent-development.mdx`
