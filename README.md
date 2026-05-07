<img src="https://hxoqk4a8w8.ufs.sh/f/ZzAsUSY0y2ib988DGnzbzH8IZUEXKGOAujekVWqNxQYhbBJ5" alt="Strait header" width="100%" />

# Strait

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/strait-dev/strait/badge)](https://scorecard.dev/viewer/?uri=github.com/strait-dev/strait)
[![Go Report Card](https://goreportcard.com/badge/github.com/strait-dev/strait)](https://goreportcard.com/report/github.com/strait-dev/strait)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**Open-source job orchestration in a single Go binary.**

Strait runs your background jobs and orchestrates multi-step workflows. One service, backed by PostgreSQL and Redis. No RabbitMQ, no SQS, no Kafka.

- Full run lifecycle from `queued` to terminal state, plus `dead_letter` runs for failures that exhaust their retries -- visible in a live dashboard.
- Workflow engine with branching, parallel steps, sub-workflows, approval gates, and compensation steps.
- Configurable retry strategies (exponential, linear, fixed, custom) with jitter and per-endpoint circuit breakers.
- Durable workflows that survive multi-day sleeps, with checkpoints, expected-completion tracking, and stage notifications.
- OpenTelemetry traces, Prometheus metrics, structured logs, and real-time SSE streaming.
- SDKs in [TypeScript](https://github.com/strait-dev/strait-ts), [Python](https://github.com/strait-dev/strait-python), [Go](https://github.com/strait-dev/strait-go), [Ruby](https://github.com/strait-dev/strait-ruby), and [Rust](https://github.com/strait-dev/strait-rust). Same feature set on each.
- Self-host needs nothing beyond what's in `docker-compose.selfhost.yml`.

---

## Get started

### Self-host with Docker Compose

```bash
git clone https://github.com/strait-dev/strait.git
cd strait
make selfhost
```

That starts the Strait API, dashboard, database, and supporting services on your machine. Open http://localhost:3000, sign up, and create your first job. No Stripe, no billing, no third-party accounts.

Full walkthrough and hardening guide: [`SELFHOST.md`](SELFHOST.md).

### Or deploy the dashboard to your own Cloudflare account

[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=https://github.com/strait-dev/strait)

Bun monorepos need one manual setting during the Workers Builds import (`Root directory: apps/app` + a custom build command). Full walkthrough: [`apps/app/README.md`](apps/app/README.md#deploy-to-cloudflare).

## What you get

| | Self-host (community) | Cloud ([strait.dev](https://strait.dev)) |
|---|---|---|
| Job orchestration with retries, workflows, and `dead_letter` recovery | ✓ | ✓ |
| Workflow engine with branching, rollback, and approval gates | ✓ | ✓ |
| Real-time streaming and live updates | ✓ | ✓ |
| All SDKs (TS, Python, Go, Ruby, Rust) | ✓ | ✓ |
| Dashboard UI | ✓ | ✓ |
| Tracing, metrics, logs, and live updates | ✓ | ✓ |
| Interactive API reference at `/reference` | ✓ | ✓ |
| Billing, metering, usage limits, Stripe | — | ✓ |
| Multi-region hosted orchestration | — | ✓ |
| Hosted ClickHouse reporting | — | ✓ |
| SLA + 24/7 support | — | ✓ |

Self-host is the community edition. Billing is compiled out of the dashboard image — there is no way to connect Stripe, view plan limits, or reach an upgrade screen. Your data and your users stay on your infrastructure.

---

## Documentation

| Topic | Link |
|---|---|
| Product overview | [`apps/docs/introduction.mdx`](apps/docs/introduction.mdx) |
| Choose the right path | [`apps/docs/choose-your-path.mdx`](apps/docs/choose-your-path.mdx) |
| Quickstart | [`apps/docs/quickstart.mdx`](apps/docs/quickstart.mdx) |
| Use cases | [`apps/docs/use-cases/background-jobs.mdx`](apps/docs/use-cases/background-jobs.mdx) |
| Compare Strait | [`apps/docs/compare/message-queues.mdx`](apps/docs/compare/message-queues.mdx) |
| Architecture | [`apps/docs/architecture.mdx`](apps/docs/architecture.mdx) |
| Core concepts | [`apps/docs/concepts/jobs.mdx`](apps/docs/concepts/jobs.mdx) |
| API reference | [`apps/docs/api-reference/overview.mdx`](apps/docs/api-reference/overview.mdx) |
| SDK reference | [`apps/docs/sdks/overview.mdx`](apps/docs/sdks/overview.mdx) |
| Guides | [`apps/docs/guides/production-job.mdx`](apps/docs/guides/production-job.mdx) |
| Contributor operating guide | [`AGENTS.md`](AGENTS.md) |
| Self-host walkthrough | [`SELFHOST.md`](SELFHOST.md) |

Dedicated repositories:

| Component | Repository |
|---|---|
| CLI | [strait-dev/cli](https://github.com/strait-dev/cli) |
| TypeScript SDK | [strait-dev/strait-ts](https://github.com/strait-dev/strait-ts) |
| Python SDK | [strait-dev/strait-python](https://github.com/strait-dev/strait-python) |
| Go SDK | [strait-dev/strait-go](https://github.com/strait-dev/strait-go) |
| Ruby SDK | [strait-dev/strait-ruby](https://github.com/strait-dev/strait-ruby) |
| Rust SDK | [strait-dev/strait-rust](https://github.com/strait-dev/strait-rust) |
| MCP server | [strait-dev/mcp](https://github.com/strait-dev/mcp) |

---

## Repository layout

Turborepo monorepo managed with Bun. The bits that matter:

```
apps/
  strait/   Go service — API, worker, scheduler, all in one binary
  app/      TanStack Start dashboard (React 19, Vite)
  docs/     Mintlify docs
packages/   Shared TS packages (ui, billing, config, transactional, …)
docker-compose.selfhost.yml   One-command self-host stack
SELFHOST.md                   Self-host walkthrough
AGENTS.md                     Contributor operating guide
```

Install and run workspace tasks:

```bash
bun install
bun run lint
bun run test
bun run build
```

---

## Development checks

Run OpenAPI route parity before committing docs/API changes:

```bash
cd apps/strait && go run ./scripts/check-openapi-parity
```

Run hooks:

```bash
lefthook run pre-commit
```

See [`AGENTS.md`](AGENTS.md) for the full contributor guide: tech stack, module layout, coding conventions, testing patterns, and repository workflow.

---

## License

[Apache License 2.0](LICENSE).
