<img src="https://hxoqk4a8w8.ufs.sh/f/ZzAsUSY0y2ib988DGnzbzH8IZUEXKGOAujekVWqNxQYhbBJ5" alt="Strait header" width="100%" />

# Strait

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/strait-dev/strait/badge)](https://scorecard.dev/viewer/?uri=github.com/strait-dev/strait)
[![Go Report Card](https://goreportcard.com/badge/github.com/strait-dev/strait)](https://goreportcard.com/report/github.com/strait-dev/strait)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**Open-source job orchestration in a single Go binary.**

Strait runs your background jobs and orchestrates multi-step workflows. One service, backed by PostgreSQL and Redis. No RabbitMQ, no SQS, no Kafka.

- Full run lifecycle from `queued` to terminal state, plus a review queue for runs that exhaust their retries — visible in a live dashboard.
- Workflow engine with branching, parallel steps, sub-workflows, approval gates, and compensation steps.
- Configurable retry strategies (exponential, linear, fixed, custom) with jitter and per-endpoint circuit breakers.
- Durable workflows that survive multi-day sleeps, with checkpoints, expected-completion tracking, and stage notifications.
- OpenTelemetry traces, Prometheus metrics, structured logs, and real-time SSE streaming — built in, not bolted on.
- SDKs in [TypeScript](https://github.com/strait-dev/strait-ts), [Python](https://github.com/strait-dev/strait-python), [Go](https://github.com/strait-dev/strait-go), [Ruby](https://github.com/strait-dev/strait-ruby), and [Rust](https://github.com/strait-dev/strait-rust). Same feature set on each.
- Self-host needs nothing beyond what's in `docker-compose.selfhost.yml`.

---

## Get started in 60 seconds

### Self-host with Docker Compose

```bash
git clone https://github.com/strait-dev/strait.git
cd strait
make selfhost
```

That boots PostgreSQL, Redis, Sequin, the Strait API, and the dashboard on your machine. Open http://localhost:3000, sign up, and create your first job. No Stripe, no billing, no telemetry, no third-party accounts.

Full walkthrough and hardening guide: [`SELFHOST.md`](SELFHOST.md).

### Or deploy the dashboard to your own Cloudflare account

[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=https://github.com/strait-dev/strait)

Bun monorepos need one manual setting during the Workers Builds import (`Root directory: apps/app` + a custom build command). Full walkthrough: [`apps/app/README.md`](apps/app/README.md#deploy-to-cloudflare).

---

## Let an AI agent do the setup

Paste the block below into Claude Code, Cursor, Codex, Aider, or any coding agent. It will clone Strait, bring up the self-host stack, and walk you through triggering your first job — no manual commands on your end.

~~~
You are setting up Strait, a self-hosted job orchestration platform, on my
machine. Do everything end to end without stopping to ask me for confirmation
unless something actually fails.

1. Confirm Docker and Docker Compose v2 are installed and Docker is running.
   If Docker is not running, stop and tell me to start Docker Desktop.
2. Clone https://github.com/strait-dev/strait.git to a fresh directory and cd
   into it. If the repo already exists, cd into it and `git pull`.
3. Run `make selfhost`. This generates `.env.selfhost` with random secrets,
   then brings up Postgres, Redis, Sequin, the Strait API, and the dashboard
   via `docker-compose.selfhost.yml`.
4. Wait for every service to be healthy. Poll
   `curl -sf http://localhost:8080/health`, `curl -sf http://localhost:3000/login`,
   and `docker compose -f docker-compose.selfhost.yml ps` until all containers
   report `(healthy)`. Time out after 3 minutes and report which container
   failed if so.
5. Using the REST API directly (not the dashboard), create a project, an API
   key, and a job that POSTs to https://httpbin.org/post. Trigger a run with a
   small JSON payload. Poll the run status until it reaches `completed` or
   `failed`, then print the run ID, final state, and elapsed time.
6. Print next steps: how to open the dashboard (http://localhost:3000), where
   the API reference lives (http://localhost:8080/reference), how to view logs
   (`docker compose -f docker-compose.selfhost.yml logs -f strait`), and how
   to tear the stack down (`make selfhost-down`).

Important rules:
- Use `SELFHOST.md` as the source of truth for any command I did not spell out
  above.
- Do not install billing, Stripe, or Infisical. Strait's self-host edition has
  billing compiled out — do not try to set up a paid plan or prompt me for a
  payment provider.
- Do not commit or push anything. Do not touch my global git config.
- If `make selfhost` is unavailable on my system, fall back to
  `./packages/scripts/selfhost-init.sh` + `docker compose --env-file .env.selfhost
  -f docker-compose.selfhost.yml up -d`.
- Print a single concise summary at the end with the URLs, the project/API-key/
  job IDs you created, and the command to stop the stack.
~~~

---

## What you get

| | Self-host (community) | Cloud ([strait.dev](https://strait.dev)) |
|---|---|---|
| Job orchestration with retries, workflows, and review queue | ✓ | ✓ |
| Workflow engine with branching, rollback, and approval gates | ✓ | ✓ |
| Real-time streaming and live updates | ✓ | ✓ |
| All SDKs (TS, Python, Go, Ruby, Rust) | ✓ | ✓ |
| Dashboard UI | ✓ | ✓ |
| Built-in observability (tracing, metrics, logs) | ✓ | ✓ |
| Interactive API reference at `/reference` | ✓ | ✓ |
| Billing, metering, usage limits, Stripe | — | ✓ |
| Multi-region hosted orchestration | — | ✓ |
| Advanced analytics (ClickHouse) | — | ✓ |
| SLA + 24/7 support | — | ✓ |

Self-host is the community edition. Billing is compiled out of the dashboard image — there is no way to connect Stripe, view plan limits, or reach an upgrade screen. Your data and your users stay on your infrastructure.

---

## Documentation

| Topic | Link |
|---|---|
| Product overview | [`apps/docs/introduction.mdx`](apps/docs/introduction.mdx) |
| 10-minute quickstart | [`apps/docs/quickstart.mdx`](apps/docs/quickstart.mdx) |
| Architecture deep dive | [`apps/docs/architecture.mdx`](apps/docs/architecture.mdx) |
| Core concepts | [`apps/docs/concepts/jobs.mdx`](apps/docs/concepts/jobs.mdx) |
| API reference | [`apps/docs/api-reference/overview.mdx`](apps/docs/api-reference/overview.mdx) |
| SDK reference | [`apps/docs/sdks/overview.mdx`](apps/docs/sdks/overview.mdx) |
| Guides (auth, security, performance, and more) | [`apps/docs/guides/authentication.mdx`](apps/docs/guides/authentication.mdx) |
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
AGENTS.md                     Operating guide for contributors + AI agents
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

See [`AGENTS.md`](AGENTS.md) for the full contributor guide — tech stack, module layout, coding conventions, testing patterns, and how AI agents should work in this repo.

---

## License

[Apache License 2.0](LICENSE).
