<img src="github.png" alt="Strait header" width="100%" />

# Strait

A production-grade job orchestration platform for engineering teams and AI agents.

**Everything you need in one binary.** Accept job definitions via REST API, queue runs in PostgreSQL using `SELECT FOR UPDATE SKIP LOCKED`, dispatch them via HTTP to your endpoints, and handle retries with intelligent strategies.

---

## Why Strait?

Strait solves the complexity of background job processing by combining queue, state, scheduler, and executor in a single system—no external message broker required.

- **Zero Dependencies**: No RabbitMQ, SQS, or Kafka. PostgreSQL handles queuing with lock-free concurrent workers.
- **Production-Grade Concurrency**: Go goroutines provide parallel job execution with structured panic recovery and graceful shutdown.
- **Built for AI**: SDK endpoints for logging, heartbeats, progress checkpoints, continuation, and child job spawning. Cost budgets with micro-USD precision.
- **Multi-Language SDKs**: Official SDKs for TypeScript, Python, Go, Ruby, and Rust — all with full feature parity. Authoring DSL, composition helpers, FSM state machines, and 186 API operations.
- **Workflow Orchestration**: Complex DAGs with step conditions, output transforms, template variables, and human approval gates.
- **Observability First**: OpenTelemetry tracing, Prometheus metrics, structured JSON logging, and real-time SSE streaming.

## Quick Links

| Documentation | Description |
|---------------|-------------|
| [Introduction](docs/introduction.mdx) | Product overview, key features, and getting started |
| [Quick Start](docs/quickstart.mdx) | Set up and run your first job in 10 minutes |
| [Architecture](docs/architecture.mdx) | Deep dive into internals, queue mechanics, and technology choices |
| [CLI Reference](docs/cli/overview.mdx) | 48+ commands, TUI dashboard, and shell completion |
| [API Reference](docs/api-reference/overview.mdx) | REST API endpoints for job and workflow management |
| [Concepts](docs/concepts/jobs.mdx) | Jobs, runs, workflows, scheduling, retry strategies, and cost budgets |
| [SDK Reference](docs/sdks/overview.mdx) | Official SDKs for TypeScript, Python, Go, Ruby, and Rust |
| [Guides](docs/guides/authentication.mdx) | Authentication, deployment, security, and production patterns |

## Monorepo Layout

This repository is now structured as a Turborepo monorepo managed with Bun.

- `apps/strait`: Go service (CLI + API + worker)
- `packages/typescript-sdk`: TypeScript/Node.js SDK
- `packages/python-sdk`: Python SDK
- `packages/go-sdk`: Go SDK
- `packages/ruby-sdk`: Ruby SDK
- `packages/rust-sdk`: Rust SDK

```bash
# Install workspace tooling
bun install

# Run workspace tasks via Turbo
bun run lint
bun run test
bun run build
```

## Self-Hosting

Run Strait on your own infrastructure in under a minute:

```bash
git clone https://github.com/leonardomso/strait.git
cd strait
docker compose -f docker-compose.self-host.yml up -d
```

This starts Strait, PostgreSQL, Redis, Prometheus, and Grafana. See [docs/self-hosting.md](docs/self-hosting.md) for configuration, production hardening, and the edition comparison.

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
