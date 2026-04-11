<img src="https://hxoqk4a8w8.ufs.sh/f/ZzAsUSY0y2ib988DGnzbzH8IZUEXKGOAujekVWqNxQYhbBJ5" alt="Strait header" width="100%" />

# Strait

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/strait-dev/strait/badge)](https://scorecard.dev/viewer/?uri=github.com/strait-dev/strait)

A production-grade job orchestration platform for engineering teams and AI agents.

**Everything you need in one binary.** Accept job definitions via REST API, queue runs in PostgreSQL using `SELECT FOR UPDATE SKIP LOCKED`, dispatch them via HTTP to your endpoints, and handle retries with intelligent strategies.

---

## Why Strait?

Strait solves the complexity of background job processing by combining queue, state, scheduler, and executor in a single system—no external message broker required.

- **Zero Dependencies**: No RabbitMQ, SQS, or Kafka. PostgreSQL handles queuing with lock-free concurrent workers.
- **Production-Grade Concurrency**: Go goroutines provide parallel job execution with structured panic recovery and graceful shutdown.
- **Built for AI**: SDK endpoints for logging, heartbeats, progress checkpoints, continuation, and child job spawning. Cost budgets with micro-USD precision.
- **Multi-Language SDKs**: Official SDKs for [TypeScript](https://github.com/strait-dev/strait-ts), [Python](https://github.com/strait-dev/strait-python), [Go](https://github.com/strait-dev/strait-go), [Ruby](https://github.com/strait-dev/strait-ruby), and [Rust](https://github.com/strait-dev/strait-rust) — all with full feature parity.
- **Workflow Orchestration**: Complex DAGs with step conditions, output transforms, template variables, and human approval gates.
- **Observability First**: OpenTelemetry tracing, Prometheus metrics, structured JSON logging, and real-time SSE streaming.

## Quick Links

| Documentation | Description |
|---------------|-------------|
| [Introduction](docs/introduction.mdx) | Product overview, key features, and getting started |
| [Quick Start](docs/quickstart.mdx) | Set up and run your first job in 10 minutes |
| [Architecture](docs/architecture.mdx) | Deep dive into internals, queue mechanics, and technology choices |
| [CLI](https://github.com/strait-dev/cli) | Command-line interface (dedicated repository) |
| [API Reference](docs/api-reference/overview.mdx) | REST API endpoints for job and workflow management |
| [Concepts](docs/concepts/jobs.mdx) | Jobs, runs, workflows, scheduling, retry strategies, and cost budgets |
| [SDK Reference](docs/sdks/overview.mdx) | Official SDKs for TypeScript, Python, Go, Ruby, Rust (dedicated repos) |
| [Guides](docs/guides/authentication.mdx) | Authentication, deployment, security, and production patterns |

## Monorepo Layout

This repository is structured as a Turborepo monorepo managed with Bun.

- `apps/strait`: Go service (API + worker)
The following have moved to dedicated repositories:

| Component | Repository |
|-----------|------------|
| CLI | [strait-dev/cli](https://github.com/strait-dev/cli) |
| TypeScript SDK | [strait-dev/strait-ts](https://github.com/strait-dev/strait-ts) |
| Python SDK | [strait-dev/strait-python](https://github.com/strait-dev/strait-python) |
| Go SDK | [strait-dev/strait-go](https://github.com/strait-dev/strait-go) |
| Ruby SDK | [strait-dev/strait-ruby](https://github.com/strait-dev/strait-ruby) |
| Rust SDK | [strait-dev/strait-rust](https://github.com/strait-dev/strait-rust) |
| MCP | [strait-dev/mcp](https://github.com/strait-dev/mcp) |

```bash
# Install workspace tooling
bun install

# Run workspace tasks via Turbo
bun run lint
bun run test
bun run build
```

## Self-Hosting

Pick whichever path matches how you want to run Strait.

**Option 1 — Deploy the dashboard to Cloudflare.** The API stays on your own infrastructure, the dashboard runs on your own Cloudflare account. Bun monorepos need one manual setting during import (`Root directory: apps/app` + a custom build command) — see [apps/app/README.md](apps/app/README.md#deploy-to-cloudflare) for the full walkthrough.

[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=https://github.com/strait-dev/strait)

**Option 2 — Run the full stack locally with Docker Compose.** API, dashboard, Postgres, Redis, and Sequin (CDC), all on your own hardware:

```bash
git clone https://github.com/strait-dev/strait.git
cd strait
make selfhost
```

See [SELFHOST.md](SELFHOST.md) for both paths in detail — configuration, production hardening, and the edition comparison.

## Key Features

- **13-State FSM** — Robust lifecycle management with queued, executing, completed, failed, timed_out, dead_letter
- **Workflow DAGs** — Fan-in/fan-out, step conditions, template variables, and sub-workflow nesting
- **Smart Retry** — Exponential, linear, fixed, or custom per-attempt delays with ±20% jitter
- **RBAC & Scoped API Keys** — Project roles (admin, operator, viewer, custom), API key scopes, and actor identity tracking
- **Atomic Versioning** — Version snapshots, unique version IDs (nanoid), and configurable version policies (pin/latest/minor)
- **Tags Everywhere** — Key-value tags on jobs, workflows, and runs with GIN-indexed filtering
- **Audit Trail** — Every mutation records `created_by`/`updated_by` with actor identity from your auth provider
- **Cost Budgets** — Per-run and daily project limits with AI model usage tracking
- **Real-Time CDC** — Postgres WAL change capture via Sequin for instant event notifications
- **SDK Endpoints** — Specialized endpoints for logging, heartbeats, progress, and continuation. Official SDKs in 5 languages.
- **Webhooks** — HMAC-SHA256 signed webhooks with automatic retries and dead letter queue
- **Health Scoring** — Aggregate metrics for success rate, timeout rate, and latency stability
- **Dead Letter Queue** — Isolate permanently failed runs for inspection and replay
- **Job Chaining** — Auto-trigger downstream jobs on completion or failure with JSONPath payload mapping and max chain depth enforcement
- **Compensating Transactions** — Saga-pattern rollback handlers on workflow steps; on failure, previously completed steps are compensated in reverse topological order
- **Durable Workflows** — Expected completion tracking via critical-path analysis, stage notifications on step transitions, and reaper-safe long-running sleep/wait steps
- **Canary Deploys** — Gradual traffic ramping between workflow versions with auto-promote/rollback based on failure rate and P99 latency thresholds
- **Workflow Simulator** — Dry-run, sandbox, and failure-injection simulation modes with DAG visualization, cost estimates, and condition evaluation
- **Workflow Test Suites** — Define tests with mock endpoints and assertions (step status, output matching, duration bounds) with JUnit XML output for CI
- **Visual Debugger** — Step-by-step timeline with input/output inspection, cost attribution, data flow tracking, and cross-run comparison

## Development Checks

Run OpenAPI route parity manually before committing docs/API changes:

```bash
cd apps/strait && go run ./scripts/check-openapi-parity
```

Then run hooks/checks:

```bash
lefthook run pre-commit
```

## Project Status

[![Go Report Card](https://goreportcard.com/badge/github.com/leonardomso/strait)](https://goreportcard.com/report/github.com/leonardomso/strait)
[![Tests](https://github.com/leonardomso/strait/workflows/ci.yaml/badge.svg)](https://github.com/leonardomso/strait/actions)

## License

[MIT License](LICENSE)

---

**Ready to get started?** Follow the [Quick Start Guide](docs/quickstart.mdx) and have a production-grade job orchestration running in minutes.
