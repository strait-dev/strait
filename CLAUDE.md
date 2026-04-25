# AGENTS.md

Operating guide for contributors and AI agents working on this repository. **Read this before making changes.**

If instructions conflict, use this priority order:
1. Direct user request
2. This file
3. Existing code patterns
4. Personal preference

The same content lives in `CLAUDE.md` — keep both files in sync.

---

## 1. What Strait is

Strait is a job execution and workflow orchestration platform shipped as a single Go binary. PostgreSQL is the source of truth and the queue (`SELECT ... FOR UPDATE SKIP LOCKED`); Redis powers pub/sub and SSE; container workloads run on Docker or Kubernetes. The binary runs in `api`, `worker`, or `all` mode. Two editions (community and cloud) are selected at compile time via Go build tags.

Read first:
- `README.md`
- `apps/docs/introduction.mdx` — feature overview
- `apps/docs/quickstart.mdx` — 10-minute setup
- `apps/docs/architecture.mdx` — internals and design rationale
- `apps/docs/development/technology-choices.mdx` — why each library
- `SELFHOST.md` — self-hosted deployment

---

## 2. Tech stack

- **Language**: Go 1.26 (toolchain 1.26.2), module `strait`, repo is a Bun + Turbo monorepo
- **HTTP**: `go-chi/chi/v5` + `danielgtaylor/huma/v2` (OpenAPI generation)
- **Database**: PostgreSQL via `jackc/pgx/v5` — no ORM. Migrations are embedded SQL.
- **Cache / pub-sub**: `redis/go-redis/v9`, `eko/gocache`, `maypok86/otter`
- **Concurrency**: `sourcegraph/conc`, `alitto/pond/v2`, `failsafe-go` for retries / circuit breakers
- **Container runtimes**: `k8s.io/client-go`, Docker, BuildKit
- **Analytics (optional)**: `ClickHouse/clickhouse-go/v2`
- **Observability**: OpenTelemetry, Prometheus, Pyroscope, Sentry
- **Helpers**: `samber/lo`, `samber/oops`, `samber/slog-multi`
- **CLI internals**: `spf13/cobra` (the user-facing CLI lives in [strait-dev/cli](https://github.com/strait-dev/cli))
- **JWT**: `golang-jwt/jwt/v5`
- **Cloud-only billing**: `stripe/stripe-go/v82`
- **Tests**: `testcontainers-go` for real Postgres / Redis in integration tests
- **Tooling**: golangci-lint, lefthook, Biome, govulncheck, gitleaks

Runtime dependencies (see `apps/strait/docker-compose.yml`): PostgreSQL 18, Redis 8, Sequin (CDC). Self-host stack at the repo root: `docker-compose.selfhost.yml`.

---

## 3. Repository layout

Monorepo. Top level:

- `apps/strait/` — the Go server (this is where most work happens)
- `apps/docs/` — Mintlify docs (`.mdx` + `docs.json` nav)
- `apps/website/` — marketing site
- `apps/app/` — web app
- `packages/` — shared TS packages (`ui`, `billing`, `config`, `deploy`, `monitoring`, `transactional`)
- `.github/workflows/` — CI
- `lefthook.yml` — git hooks

Inside `apps/strait/`:

- `cmd/strait/` — entrypoint, server wiring, migration runner (`main.go`, `server.go`, `services.go`, `migrate.go`)
- `migrations/` — embedded SQL migrations
- `schemas/strait.json` — generated OpenAPI spec
- `k8s/` — example manifests + Grafana dashboards
- `internal/` — application code:

| Package | Purpose |
|---|---|
| `api/` | HTTP handlers (chi + Huma), auth, RBAC, idempotency, request validation |
| `worker/` | Dequeue loop, executor pool, dispatch, graceful drain |
| `dispatcher/` | Routes runs to the correct compute runtime |
| `workflow/` | DAG engine, step progression, conditionals, compensation/saga, durable waits |
| `compute/` | K8s / Docker / HTTP runtimes, warm pool, cost estimation, signal classification |
| `queue/` | Lock-free claim, concurrency control |
| `scheduler/` | Cron, reaper, retention, pool pruner background loops |
| `store/` | Raw `pgx/v5` data access, one file per table area |
| `domain/` | Types, FSM states, edition gating |
| `clickhouse/` | Optional analytics export, schema, exporter |
| `webhook/` | HMAC delivery, retry, circuit breaker, dead-letter queue |
| `cdc/` | Sequin-backed change data capture |
| `pubsub/` | Redis (prod) / in-memory (test) pub/sub for SSE |
| `logdrain/` | Datadog / Splunk / HTTP log forwarding |
| `eventfilter/` | Event-trigger matching rules |
| `notification/` | Slack / email / PagerDuty channels |
| `health/` | Health checks and scoring |
| `bundle/` | Code bundle upload + deploy pipeline |
| `registry/` | Container registry abstraction (ECR, Docker Registry v2) |
| `objectstore/` | S3-compatible storage |
| `cache/` | Multi-layer caching primitives |
| `dbscan/` | Anomaly detection on run metrics |
| `debug/` | Debug bundle generation |
| `crypto/` | Secret encryption, API key hashing, HMAC signing |
| `ratelimit/` | Per-job, per-IP, per-project limits |
| `telemetry/` | OTel tracing, Prometheus metrics, Pyroscope, Sentry |
| `errors/` | Typed error helpers |
| `httputil/` | HTTP client helpers (SSRF guards, timeouts) |
| `config/` | Env var loading via `aconfig` |
| `billing/` | Quotas, Stripe (cloud only) |
| `loadtest/` | Throughput / chaos / endurance harness |
| `e2e/` | End-to-end test suite |
| `testutil/` | Test DB, factories, assertion helpers |

---

## 4. Features and where to read more

Map of platform capabilities. Each links to the doc that explains it in depth — read these instead of guessing.

**Execution and runs**
- Jobs and 13-state run FSM — `apps/docs/concepts/jobs.mdx`, `apps/docs/concepts/runs.mdx`
- Managed execution (K8s/Docker/HTTP runtimes, warm pool) — `apps/docs/concepts/managed-execution.mdx`
- Versioning and policies — `apps/docs/concepts/versioning.mdx`
- Job chaining — `apps/docs/concepts/job-chaining.mdx`
- Batch operations — `apps/docs/concepts/batch-operations.mdx`

**Workflows**
- DAG runtime, sub-workflows, approvals — `apps/docs/concepts/workflows.mdx`, `apps/docs/concepts/dag-runtime.mdx`
- Compensating transactions (saga) — `apps/docs/concepts/compensating-transactions.mdx`
- Durable / long-running workflows — `apps/docs/concepts/durable-workflows.mdx`
- Workflow simulator — `apps/docs/concepts/workflow-simulator.mdx`
- Workflow test suites — `apps/docs/concepts/workflow-test-suites.mdx`
- Workflow debugger — `apps/docs/concepts/workflow-debugger.mdx`

**Resilience and operations**
- Retry strategies — `apps/docs/concepts/retry-strategies.mdx`
- Adaptive concurrency, resilience patterns — `apps/docs/concepts/adaptive-concurrency.mdx`, `apps/docs/concepts/resilience.mdx`
- Canary deployments — `apps/docs/concepts/canary-deployments.mdx`
- Cost budgets — `apps/docs/concepts/cost-budgets.mdx`
- Environments (dev/stg/prd) — `apps/docs/concepts/environments.mdx`

**Triggers and events**
- Scheduling (cron) — `apps/docs/concepts/scheduling.mdx`
- Event triggers and sources — `apps/docs/concepts/event-triggers.mdx`, `apps/docs/concepts/event-sources.mdx`
- Outbound webhooks and subscriptions — `apps/docs/concepts/webhooks.mdx`, `apps/docs/concepts/webhook-subscriptions.mdx`
- CDC — `apps/docs/concepts/cdc.mdx`

**Observability**
- ClickHouse analytics — `apps/docs/concepts/clickhouse-analytics.mdx`
- Audit logging — `apps/docs/concepts/audit-logging.mdx`
- Log drains — `apps/docs/concepts/log-drains.mdx`
- Monitoring and alerts — `apps/docs/operations/monitoring-and-alerts.mdx`

**Security and operational guides**
- Authentication — `apps/docs/guides/authentication.mdx`
- RBAC — `apps/docs/guides/rbac.mdx`
- OIDC — `apps/docs/guides/oidc.mdx`
- API keys and rotation — `apps/docs/guides/api-key-rotation.mdx`
- Security model — `apps/docs/guides/security.mdx`
- Workflow approvals — `apps/docs/guides/workflow-approvals.mdx`
- Idempotency — `apps/docs/guides/idempotency.mdx`
- SDK integration — `apps/docs/guides/sdk-integration.mdx`
- Performance tuning — `apps/docs/guides/performance-tuning.mdx`
- Capacity planning — `apps/docs/guides/capacity-planning.mdx`
- Deployment — `apps/docs/guides/deployment.mdx`
- DAG operations playbook — `apps/docs/guides/dag-operations-playbook.mdx`
- Debug bundles — `apps/docs/guides/debug-bundles.mdx`

**Reference**
- API: `apps/docs/api-reference/` + `apps/strait/schemas/strait.json`
- Configuration / env vars: `apps/docs/configuration/environment-variables.mdx` + `.env.example`
- Database schema: `apps/docs/development/database-schema.mdx`
- Architecture deep dive: `apps/docs/architecture.mdx`
- Tech choices rationale: `apps/docs/development/technology-choices.mdx`
- Contributing: `apps/docs/development/contributing.mdx`
- Testing: `apps/docs/development/testing.mdx`

If you have unresolved questions about scope, schema, API contracts, or user-facing semantics after reading the relevant docs, **ask the user before implementing.** Do not assume.

---

## 5. Editions

Edition is set at compile time via Go build tags. The `STRAIT_EDITION` env var is ignored.

- **Community** (`go build`): self-hosted, open source. Docker + K8s runtimes, no billing.
- **Cloud** (`go build -tags cloud`): SaaS at strait.dev. All features, Stripe billing, multi-region.

`domain.ParseEdition()` returns the compile-time edition. See `apps/strait/internal/domain/edition_community.go` and `edition_cloud.go`. Cloud-only files use `//go:build cloud`. Docker: `docker build --build-arg BUILD_TAGS=cloud`.

---

## 6. Local setup

```bash
# 1. Start dependencies
cd apps/strait && docker compose up -d

# 2. Required env (see .env.example for the full list)
export DATABASE_URL=postgres://strait:strait@localhost:5432/strait?sslmode=disable
export REDIS_URL=redis://localhost:6379
export INTERNAL_SECRET=<32+ chars>
export JWT_SIGNING_KEY=<32+ chars>

# 3. Run
cd apps/strait && go run ./cmd/strait --mode all
```

Migrations are embedded and auto-applied on startup. To create a new pair manually:

```bash
cd apps/strait && go run ./cmd/strait migrate create <name>
cd apps/strait && go run ./cmd/strait migrate up
cd apps/strait && go run ./cmd/strait migrate status
```

Migration safety:
- Never edit historical migrations.
- Always ship `up` and `down` together (`NNNNNN_name.up.sql` / `NNNNNN_name.down.sql`).

---

## 7. Validation commands

Run before pushing:

```bash
cd apps/strait && go build ./...
cd apps/strait && go build -tags cloud ./...
cd apps/strait && go test ./...
cd apps/strait && go test -race ./...
cd apps/strait && golangci-lint run --timeout=5m ./...
```

When touching DB / queue / workflow / scheduler / pubsub:

```bash
cd apps/strait && go test -tags integration ./...
```

Lefthook is mandatory:

```bash
lefthook install
```

Hook groups (`lefthook.yml`):
- `pre-commit`: gitleaks (secrets) + Biome (TS/JS format)
- `pre-push` (every branch, parallel): `manypkg:check`, `biome check`, `typecheck`, `bun run go:lint` (cached + incremental vs `origin/master`)
- `commit-msg`: enforces Conventional Commits

**Never bypass hooks** with `--no-verify` or similar. Fix the underlying failure instead.

---

## 8. Planning protocol and implementation workflow (mandatory)

Canonical workflow for any non-trivial change. Follow this whenever you plan or implement work, and **always** when entering plan mode.

### 8.1 Before creating a plan

1. Understand the request and constraints.
2. Read the relevant docs (section 4) and the nearest code paths.
3. Identify every unresolved question that could affect scope, schema, API contracts, or user-facing semantics.
4. **Ask all unresolved questions and wait for answers before drafting the plan.** Never proceed on assumptions.

### 8.2 Plan shape

Every plan must be:
- **Detailed and complete** — covers the full implementation end-to-end, not just the first slice.
- **Separated into phases** — each phase is a coherent, independently verifiable unit of work.
- **Presented inline in the conversation** — never write plan files (`PLAN.md`, design docs, etc.) unless the user explicitly asks for one.

For every phase, specify: goal, files to change or create, tests to add/update, validation commands.

### 8.3 Per-phase execution loop

Once the plan is approved, execute each phase in order. For every phase:

1. Implement the phase exactly as planned.
2. Run the full validation suite:
   ```bash
   cd apps/strait && go build ./...
   cd apps/strait && go test ./...
   cd apps/strait && go test -race ./...
   cd apps/strait && golangci-lint run --timeout=5m ./...
   ```
   Add `cd apps/strait && go test -tags integration ./...` when DB / queue / workflow / scheduler / pubsub behavior is touched.
3. If a check fails:
   - Fix small, local issues immediately and re-run until everything is green.
   - **If the failure requires significantly more work than the phase contemplated** (architectural change, schema rework, cross-cutting refactor, newly ambiguous requirement) — **stop, report the situation to the user, and wait for direction.** Do not push through.
4. Once **all** checks pass, commit the phase's changes with a Conventional Commit message (see section 10). Never use `--no-verify` or bypass lefthook.
5. Proceed to the next phase **without asking permission**. Implement the entire plan to completion in one go.

### 8.4 Plan adherence

- Always follow the plan as agreed. If reality forces a deviation, stop, explain, and wait for the user to confirm before proceeding.
- Never silently expand scope mid-phase.
- Never skip validation or commit gates.
- Tests live in the same phase as the behavior change — never deferred to a later phase.
- Update docs / OpenAPI (`apps/strait/schemas/strait.json`) / `.env.example` in the same phase that introduces the change.

---

## 9. Engineering rules (non-negotiable)

1. **No ORM.** Raw SQL with `pgx/v5` patterns in `internal/store`.
2. **Structured concurrency.** Use `sourcegraph/conc` and `alitto/pond/v2`. No casual goroutine fan-out.
3. **Errors.** Wrap with `%w` and contextual messages. Use `samber/oops` for stack traces in critical paths.
4. **Helpers.** Prefer `samber/lo` where it improves readability.
5. **Tests.** Use `apps/strait/internal/testutil` helpers. Meaningful assertions, not just "no error".
6. **Worker / pool consistency.** Reuse existing patterns in `internal/worker`. Don't fork dispatch logic.
7. **No emojis.** In code, comments, logs, docs, commits, PR text — anywhere.
8. **Observability is load-bearing.** Preserve traces / metrics / logs in critical paths (worker, queue, workflow, scheduler).
9. **Auth boundaries.** Don't weaken SSRF guards, internal management secret flow, or SDK JWT run-token flow.
10. **Graceful shutdown.** Don't alter shutdown behavior in ways that risk in-flight job loss.

---

## 10. Commits and PRs

### Conventional Commits (mandatory)

Format: `type(scope): summary`

Allowed types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`, `revert`. Use `!` for breaking changes and explain in the body. lowercase types, imperative summary, scope when useful.

Examples:
- `feat(worker): add retry jitter cap for webhook dispatch`
- `fix(queue): prevent dequeue race on stale heartbeat`
- `test(workflow): add regression for fan-in completion`

**Never** add:
- "Co-Authored-By" lines
- "Generated with Claude Code" or any AI attribution
- Vague messages (`update`, `misc`, `fix stuff`)

### PR descriptions

Substantive, not boilerplate. Include:
- **Summary** — what the PR does in plain language
- **Why** — context and motivation
- **What changed** — grouped by area
- **Validation** — exact commands run + outcomes
- **Tests added or updated** and why
- **Docs / OpenAPI / env impact** (or explicit "none")
- **Risks and follow-ups**

Never claim validation without listing the commands. Never paste generic boilerplate that could apply to any PR. Never add AI attribution footers.

---

## 11. DOs and DON'Ts

### Do
- Confirm assumptions when requirements are ambiguous.
- Follow existing package boundaries and naming patterns.
- Keep changes small, focused, and reversible.
- Add tests for new behavior; regression tests for bug fixes.
- Maintain backward compatibility unless the user requests breakage.
- Update OpenAPI (`apps/strait/schemas/strait.json`), docs, and `.env.example` when the surface changes.

### Don't
- Guess business rules, API contracts, or schema intent.
- Refactor unrelated code in the same PR.
- Ship behavior without tests.
- Bypass failing tests, lint, or hooks.
- Weaken auth, RBAC, or SSRF guards.
- Mark work as complete without validation evidence.

---

## 12. Definition of done

A change is done only when:

1. Code builds for both editions (`go build ./...` and `go build -tags cloud ./...`).
2. Relevant tests pass (unit, race, integration when applicable).
3. Lint passes.
4. Docs / OpenAPI / `.env.example` updated for behavior changes.
5. Migration rules followed (paired up/down, never edit history).
6. Summary provided: what changed, why, how validated.

---

When in doubt, prefer established project patterns over novelty, ask clarifying questions early, and keep changes explicit and verifiable.
