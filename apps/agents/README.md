# `@strait/agents`

`apps/agents` is the runtime package for managed Strait agents.

It serves two roles:

1. the local execution target used during local-first development
2. the Cloudflare Workers package used for production runtime, dispatch, and outbound sandboxing

This package is intentionally small at the workspace root. The runtime behavior lives in `src/`, while deployment shape is defined by the Wrangler configs.

## What This Package Contains

- `src/core.ts`
  - shared runtime logic used by both the local and Cloudflare paths
- `src/index.ts`
  - local CLI entrypoint
- `src/worker.ts`
  - Cloudflare runtime Worker entrypoint
- `src/dispatch.ts`
  - Cloudflare dispatch Worker entrypoint
- `src/outbound.ts`
  - Cloudflare outbound Worker entrypoint for egress policy enforcement
- `wrangler.jsonc`
  - runtime Worker config
- `wrangler.dispatch.jsonc`
  - dispatch Worker config
- `wrangler.outbound.jsonc`
  - outbound Worker config
- `scripts/cloudflare-smoke.sh`
  - operator smoke test for staging once a Workers for Platforms account is available

## Architecture

### Local Path

The local path is the default development loop.

1. Strait creates an agent run in Go.
2. The Go control plane builds a dispatch envelope.
3. `apps/agents` runs locally through `src/index.ts`.
4. The shared runtime core emits runtime events.
5. Those events are forwarded back to Strait callback endpoints and persisted in:
   - `job_runs`
   - `run_usage`
   - `run_checkpoints`
   - `run_tool_calls`

### Cloudflare Path

The production Cloudflare path uses the same runtime core with different entrypoints.

1. Strait deploys a versioned runtime Worker for an agent deployment.
2. Strait calls the dispatch Worker over HTTPS.
3. The dispatch Worker resolves the target runtime Worker from the dispatch namespace.
4. The runtime Worker executes the same contract used locally.
5. Runtime events are forwarded back to Strait callback endpoints.
6. Any network egress can be evaluated by the outbound Worker according to sandbox policy.

## Runtime Contract

The runtime receives a `DispatchEnvelope` and emits newline-delimited JSON `RuntimeEvent` records.

The contract covers:

- checkpoints
- usage reports
- tool-call telemetry
- stream chunks
- terminal completion
- terminal failure

This keeps local and Cloudflare execution aligned. The runtime never writes directly to Postgres. It only reports through the Go API callback surface.

## Supported Execution Modes

The shared runtime core currently supports several deterministic modes for local development and tests:

- `generic`
  - default echo-style payload handling
- `dynamic_planner`
  - emits a validated `dynamic_steps` envelope for workflow expansion
- `worker`
  - emits a structured worker finding payload
- `synthesizer`
  - emits a synthesized summary payload

There are also adversarial scenarios used by tests and dogfooding:

- failure mode
- invalid JSON mode
- disconnect mode
- duplicate checkpoint mode

These keep the run lifecycle and callback handling easy to verify without requiring live model providers.

## Local Development

Run the local CLI runtime:

```bash
cd apps/agents
bun run dev
```

Run the package tests:

```bash
cd apps/agents
bun run test
bun run typecheck
bun run biome:lint
```

The local runtime is normally invoked by the Go control plane, not manually. For the full local platform loop, see:

- `apps/docs/concepts/agents.mdx`
- `apps/docs/guides/local-agent-development.mdx`

## Building Cloudflare Workers

Build all three Worker bundles:

```bash
cd apps/agents
bun run build
```

Build individual bundles:

```bash
bun run build:runtime
bun run build:dispatch
bun run build:outbound
```

These produce dry-run deploy outputs under `dist/`.

The Go control plane prefers the built runtime bundle at `dist/runtime/worker.js` when deploying agent Workers. If the built artifact is missing, it falls back to the embedded runtime snapshot in:

- `apps/strait/internal/agents/runtime_worker_bundle.js`

## Deploying Shared Cloudflare Workers

The dispatch and outbound Workers are shared infrastructure and can be deployed directly from this package:

```bash
cd apps/agents
bun run deploy:dispatch
bun run deploy:outbound
```

The runtime Worker is not deployed manually for each agent. Strait uploads a versioned runtime Worker as part of agent deployment.

## Sandbox / Outbound Worker

The first production sandbox implementation is outbound-worker based.

The outbound Worker:

- blocks private-network destinations
- blocks non-HTTP protocols
- enforces allowlists
- annotates responses with policy metadata

This makes blocked network access visible to the control plane instead of failing silently.

## Validation Expectations

When changing this package, run:

```bash
cd apps/agents
bun run test
bun run typecheck
bun run biome:lint
```

And for end-to-end confidence, also run the Strait backend checks:

```bash
cd apps/strait
go test ./...
go test -race ./...
go test -tags integration ./...
```

## Related References

- `apps/docs/concepts/agents.mdx`
- `apps/docs/sdks/agents.mdx`
- `apps/docs/guides/local-agent-development.mdx`
- `apps/docs/guides/cloudflare-agents-productionization.mdx`
