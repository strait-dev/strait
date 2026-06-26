<img src=".github/github.jpg" alt="Strait" width="100%" />

<h1 align="center">Strait</h1>

<p align="center"><strong>Cloud-first job orchestration with a community self-hosted edition.</strong></p>

<p align="center">
  <a href="https://scorecard.dev/viewer/?uri=github.com/strait-dev/strait"><img src="https://api.scorecard.dev/projects/github.com/strait-dev/strait/badge" alt="OpenSSF Scorecard" /></a>
  <a href="https://goreportcard.com/report/github.com/strait-dev/strait"><img src="https://goreportcard.com/badge/github.com/strait-dev/strait" alt="Go Report Card" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License: Apache 2.0" /></a>
</p>

Strait runs background jobs and multi-step workflows. Strait Cloud hosts the control plane at `https://api.strait.dev`; your job code stays in your infrastructure. The community edition runs the same orchestration engine as a single Go binary backed by PostgreSQL, Redis, and Sequin CDC.

- **Jobs and runs.** Trigger work over HTTP or a connected worker, then watch each run move from `queued` to `completed`, or to `dead_letter` when its retries run out, in a live dashboard.
- **Your code, your infrastructure.** Strait never runs your code itself. It reaches the endpoint you expose over HTTP, or a long-lived worker you connect over gRPC, and streams the results back.
- **Workflows.** Compose multi-step workflows with branching, parallel steps, sub-workflows, human approval gates, and compensation steps that roll back partial work when something fails.
- **Durable execution.** Workflows survive process restarts and multi-day sleeps, with checkpoints, expected-completion tracking, and stage notifications.
- **Retries and resilience.** Exponential, linear, fixed, or custom backoff with jitter, per-endpoint circuit breakers, and adaptive concurrency that backs off under load.
- **Scheduling and events.** Cron schedules with timezone support, plus event triggers and inbound event sources that start work when something happens elsewhere.
- **Failure recovery.** Inspect a failed run, fix the cause, and replay it. Dead-letter runs are kept for review instead of silently dropped.
- **Observability built in.** OpenTelemetry traces, Prometheus metrics, structured logs, and real-time SSE streaming, with optional ClickHouse analytics, audit logs, and log drains.
- **SDKs and tooling.** Official SDKs for [TypeScript](https://github.com/strait-dev/strait-ts), [Python](https://github.com/strait-dev/strait-python), [Go](https://github.com/strait-dev/strait-go), [Ruby](https://github.com/strait-dev/strait-ruby), and [Rust](https://github.com/strait-dev/strait-rust) with the same feature set on each, plus a [CLI](https://github.com/strait-dev/cli) and an [MCP server](https://github.com/strait-dev/mcp).
- **Cloud or community self-hosting.** Start on Strait Cloud for the shortest production path, or run the community stack yourself when you need local evaluation, residency control, or air-gapped operation.

---

## Get Started

### Strait Cloud

Use Strait Cloud when you want the fastest path to a production job. Strait operates the API, scheduler, worker plane, storage, upgrades, and billing controls.

1. Sign in at [app.strait.dev](https://app.strait.dev).
2. Create an organization and project.
3. Create a project API key.
4. Follow the [Cloud quickstart](apps/docs/quickstart.mdx) to create a job and trigger a run.

```bash
export STRAIT_BASE_URL=https://api.strait.dev
export STRAIT_API_KEY=strait_replace_with_your_key
export STRAIT_PROJECT_ID=proj_replace_with_your_project_id
```

### Community Self-Host

Use community self-hosting when you need local evaluation, compliance controls, residency control, or full ownership of the runtime.

Prerequisites: Docker with Compose v2, Git, and a POSIX shell.

```bash
git clone https://github.com/strait-dev/strait.git
cd strait
make selfhost
```

That starts the Strait API, dashboard, PostgreSQL, Redis, and Sequin on your machine. Open http://localhost:3000, sign up, and create your first job. Everything runs locally, with no Stripe, billing, or hosted Strait account required.

Full walkthrough and hardening guide: [`SELFHOST.md`](SELFHOST.md).

### Deploy The Dashboard Separately

[![Deploy with Vercel](https://vercel.com/button)](https://vercel.com/new/clone?repository-url=https%3A%2F%2Fgithub.com%2Fstrait-dev%2Fstrait&project-name=strait-app&repository-name=strait-app&root-directory=apps%2Fapp&install-command=cd+..%2F..+%26%26+bun+install+--frozen-lockfile&build-command=cd+..%2F..+%26%26+cd+apps%2Fapp+%26%26+bun+run+build%3Avercel&env=AUTH_DATABASE_URL%2CBETTER_AUTH_URL%2CBETTER_AUTH_SECRET%2CSTRAIT_API_URL%2COIDC_ISSUER%2COIDC_AUDIENCE%2COIDC_PRIVATE_KEY_PEM)

The dashboard supports a portable Node/Docker target and a managed Vercel target. Full walkthrough: [`apps/app/README.md`](apps/app/README.md#deploy-to-vercel).

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
| Billing, metering, usage limits, Stripe | | ✓ |
| Multi-region hosted orchestration | | ✓ |
| Hosted ClickHouse reporting | | ✓ |
| SLA + 24/7 support | | ✓ |

Cloud is the default hosted product. Community self-hosting runs the open-source edition. Billing is compiled out of the community dashboard image, so there is no Stripe connection, plan limit, or upgrade screen, and your data and users stay on your infrastructure.

---

## Documentation

| Topic | Link |
|---|---|
| Choose the right path | [`apps/docs/choose-your-path.mdx`](apps/docs/choose-your-path.mdx) |
| Cloud quickstart | [`apps/docs/quickstart.mdx`](apps/docs/quickstart.mdx) |
| Product overview | [`apps/docs/introduction.mdx`](apps/docs/introduction.mdx) |
| Self-host walkthrough | [`SELFHOST.md`](SELFHOST.md) |
| Use cases | [`apps/docs/use-cases/background-jobs.mdx`](apps/docs/use-cases/background-jobs.mdx) |
| Compare Strait | [`apps/docs/compare/message-queues.mdx`](apps/docs/compare/message-queues.mdx) |
| Architecture | [`apps/docs/architecture.mdx`](apps/docs/architecture.mdx) |
| Core concepts | [`apps/docs/concepts/jobs.mdx`](apps/docs/concepts/jobs.mdx) |
| API reference | [`apps/docs/api-reference/overview.mdx`](apps/docs/api-reference/overview.mdx) |
| SDK reference | [`apps/docs/sdks/overview.mdx`](apps/docs/sdks/overview.mdx) |
| Guides | [`apps/docs/guides/production-job.mdx`](apps/docs/guides/production-job.mdx) |
| Contributor operating guide | [`AGENTS.md`](AGENTS.md) |

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

```text
apps/
  strait/   Go service. API, worker, scheduler, all in one binary.
  app/      TanStack Start dashboard (React 19, Vite).
  docs/     Mintlify docs.
packages/   Shared TS packages (ui, billing, config, transactional, ...).
docker-compose.base.yml       Shared runtime stack.
docker-compose.selfhost.yml   Self-host overrides and dashboard profile.
SELFHOST.md                   Self-host walkthrough.
AGENTS.md                     Contributor operating guide.
```

Install and run workspace tasks:

```bash
bun install
bun run lint
bun run test
bun run build
```

---

## Development Checks

Run source-backed documentation checks before committing Markdown changes:

```bash
bun run --cwd apps/docs lint
```

Run OpenAPI route parity before committing API changes:

```bash
cd apps/strait && go run ./scripts/check-openapi-parity
```

Run hooks:

```bash
lefthook run pre-commit
```

See [`AGENTS.md`](AGENTS.md) for the full contributor guide: tech stack, module layout, coding conventions, testing patterns, and repository workflow.

---

## Support And Security

Use [GitHub Discussions](https://github.com/strait-dev/strait/discussions) for questions, setup help, and design discussion. Use [GitHub Issues](https://github.com/strait-dev/strait/issues) for reproducible bugs and public feature requests.

Do not open public issues for vulnerabilities, exposed credentials, or reports that include private data. Email security reports to [security@strait.dev](mailto:security@strait.dev). See [`SECURITY.md`](SECURITY.md) for the disclosure process and response targets.

Cloud customers should use their contracted support channel for production incidents and account-specific help.

---

## License

[Apache License 2.0](LICENSE).
