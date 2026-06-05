# Contributor Operating Guide

Operating guide for contributors and AI agents working on this repository. **Read this before making changes.**

If instructions conflict, use this priority order:
1. Direct user request
2. This file
3. Existing code patterns
4. Personal preference

The same guide lives in both `AGENTS.md` and `CLAUDE.md`. Keep them in sync: any edit to one must be mirrored in the other.

---

## 1. What Strait is

Strait is a job orchestration and workflow platform shipped as a single Go binary. PostgreSQL is the source of truth. Strait's internal queue uses a vendored and modified SQL snapshot of PgQue as the ready-event log, while Strait owns run state, execution ownership, retries, workflows, workers, observability, and APIs. Redis powers pub/sub and SSE. Strait does not run user code itself: job code lives on the customer's infrastructure and is reached either through an HTTP endpoint Strait POSTs to, or through a long-lived worker process that connects to the API over gRPC and streams runs back. The binary runs in `api`, `worker`, or `all` mode. The two editions (community and cloud) are selected at compile time through Go build tags.

Read first:
- `README.md`
- `apps/docs/introduction.mdx`: feature overview
- `apps/docs/quickstart.mdx`: first setup
- `apps/docs/architecture.mdx`: internals and design rationale
- `SELFHOST.md`: self-hosted deployment

---

## 2. Tech stack

- **Language**: Go 1.26.4, module `strait`, in a Bun + Turbo monorepo
- **HTTP**: `go-chi/chi/v5` with `danielgtaylor/huma/v2` (OpenAPI generation)
- **Database**: PostgreSQL through `jackc/pgx/v5`, no ORM. Migrations are embedded SQL.
- **Cache and pub-sub**: `redis/go-redis/v9`, `eko/gocache`, `maypok86/otter`
- **Concurrency**: `sourcegraph/conc`, `alitto/pond/v2`, `failsafe-go` for retries and circuit breakers
- **Worker plane (gRPC)**: `google.golang.org/grpc` with `protobuf` for bidirectional streaming between the API and connected workers
- **Analytics (optional)**: `ClickHouse/clickhouse-go/v2`
- **Monitoring**: OpenTelemetry, Prometheus, Pyroscope, Sentry
- **Helpers**: `samber/lo`, `samber/oops`, `samber/slog-multi`
- **CLI internals**: `spf13/cobra` (the user-facing CLI lives in [strait-dev/cli](https://github.com/strait-dev/cli))
- **JWT**: `golang-jwt/jwt/v5`
- **Cloud-only billing**: `stripe/stripe-go/v82`
- **Tests**: `testcontainers-go` for real Postgres and Redis in integration tests
- **Tooling**: golangci-lint, lefthook, Biome, govulncheck, gitleaks, buf, zizmor

Runtime dependencies (see `apps/strait/docker-compose.yml`): PostgreSQL 18, Redis 8, Sequin v0.14.6 (CDC). The self-host stack lives at the repo root in `docker-compose.selfhost.yml`.

---

## 3. Repository layout

Monorepo. Top level:

- `apps/strait/`: the Go server (this is where most work happens)
- `apps/docs/`: Mintlify docs (`.mdx` files plus `docs.json` nav)
- `apps/app/`: web app dashboard
- `packages/`: shared TS packages (`ui`, `billing`, `config`, `configs`, `deploy`, `monitoring`, `scripts`, `transactional`)
- `.github/workflows/`: CI
- `lefthook.yml`: git hooks
- Marketing site lives in its own repo: <https://github.com/strait-dev/website>

Inside `apps/strait/`:

- `cmd/strait/`: entrypoint, server wiring, migration runner (`main.go`, `server.go`, `services.go`, `migrate.go`)
- `migrations/`: embedded SQL migrations
- `proto/`: gRPC worker-plane protobuf definitions (linted by `buf`)
- `schemas/strait.json`: generated OpenAPI spec
- `monitoring/`: Prometheus rules and Grafana dashboards
- `internal/`: application code:

| Package | Purpose |
|---|---|
| `api/` | HTTP handlers (chi + Huma), auth, RBAC, idempotency, request validation. Includes the gRPC worker-plane server under `api/grpc/`. |
| `worker/` | Dequeue loop, executor pool, HTTP dispatch, gRPC worker-mode dispatch, graceful drain |
| `workflow/` | Workflow engine, step progression, conditionals, compensation/saga, durable waits |
| `queue/` | Lock-free claim, concurrency control |
| `scheduler/` | Cron, reaper, retention, pool pruner background loops |
| `store/` | Raw `pgx/v5` data access, one file per table area |
| `domain/` | Types, FSM states, execution modes (`http`, `worker`), edition gating |
| `clickhouse/` | Optional analytics export, schema, exporter |
| `webhook/` | HMAC delivery, retry, circuit breaker, review queue |
| `cdc/` | Sequin-backed change data capture |
| `pubsub/` | Redis (prod) / in-memory (test) pub/sub for SSE |
| `logdrain/` | Datadog / Splunk / HTTP log forwarding |
| `eventfilter/` | Event-trigger matching rules |
| `notification/` | Slack / email / PagerDuty channels |
| `health/` | Health checks and scoring |
| `cache/` | Multi-layer caching primitives |
| `dbscan/` | Anomaly detection on run metrics |
| `bundle/` | Export and import of jobs and workflows as portable bundles (config as code) |
| `debug/` | Debug bundle generation for support and troubleshooting |
| `migrationlint/` | Safety linter for SQL migrations; flags statements that are dangerous on a live database |
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

Map of platform capabilities. Each entry links to the doc that explains it in depth; read these instead of guessing.

**Execution and runs**
- Jobs and run lifecycle: `apps/docs/concepts/jobs.mdx`, `apps/docs/concepts/runs.mdx`
- Execution modes (HTTP and gRPC worker): `apps/docs/concepts/execution-modes.mdx`
- Versioning and policies: `apps/docs/concepts/versioning.mdx`
- Job chaining: `apps/docs/concepts/job-chaining.mdx`
- Batch operations: `apps/docs/concepts/batch-operations.mdx`

**Workflows**
- Workflows, sub-workflows, approvals: `apps/docs/concepts/workflows.mdx`, `apps/docs/guides/workflow-approvals.mdx`
- Compensating transactions (saga): `apps/docs/concepts/compensating-transactions.mdx`
- Durable and long-running workflows: `apps/docs/concepts/durable-workflows.mdx`
- Workflow simulator: `apps/docs/concepts/workflow-simulator.mdx`
- Workflow test suites: `apps/docs/concepts/workflow-test-suites.mdx`
- Workflow debugger: `apps/docs/concepts/workflow-debugger.mdx`

**Resilience and operations**
- Retry strategies: `apps/docs/concepts/retry-strategies.mdx`
- Adaptive concurrency and resilience patterns: `apps/docs/concepts/adaptive-concurrency.mdx`
- Canary deployments: `apps/docs/concepts/canary-deployments.mdx`
- Cost budgets: `apps/docs/concepts/cost-budgets.mdx`
- Environments (dev/stg/prd): `apps/docs/concepts/environments.mdx`

**Triggers and events**
- Scheduling (cron): `apps/docs/concepts/scheduling.mdx`
- Event triggers and sources: `apps/docs/concepts/event-triggers.mdx`, `apps/docs/concepts/event-sources.mdx`
- Outbound webhooks and subscriptions: `apps/docs/concepts/webhooks.mdx`, `apps/docs/concepts/webhook-subscriptions.mdx`

**Monitoring**
- ClickHouse analytics: `apps/docs/concepts/clickhouse-analytics.mdx`
- Audit logging: `apps/docs/concepts/audit-logging.mdx`
- Log drains: `apps/docs/concepts/log-drains.mdx`

**Security and operational guides**
- Authentication (incl. OIDC, API key rotation): `apps/docs/guides/authentication.mdx`
- RBAC: `apps/docs/guides/rbac.mdx`
- Security model: `apps/docs/guides/security.mdx`
- Workflow approvals: `apps/docs/guides/workflow-approvals.mdx`
- Idempotency: `apps/docs/guides/idempotency.mdx`
- SDK integration: `apps/docs/guides/sdk-integration.mdx`
- Deployment: `apps/docs/guides/deployment.mdx`
- Failed run handling: `apps/docs/guides/handle-failed-runs.mdx`
- Audit events: `apps/docs/guides/audit-events.mdx`
- Event triggers guide: `apps/docs/guides/event-triggers.mdx`

**Reference**
- API: `apps/docs/api-reference/` and `apps/strait/schemas/strait.json`
- Configuration and env vars: `apps/docs/configuration/environment-variables.mdx` and `.env.example`

If you have unresolved questions about scope, schema, API contracts, or user-facing semantics after reading the relevant docs, **ask the user before implementing.** Do not assume.

---

## 5. Editions

Edition is set at compile time through Go build tags. The `STRAIT_EDITION` env var is ignored.

- **Community** (`go build`): self-hosted, open source. No billing.
- **Cloud** (`go build -tags cloud`): hosted orchestrator at strait.dev (API, Postgres, Redis, scheduler, and the gRPC worker plane). Stripe billing, multi-region, advanced analytics. Customer code still runs on customer infrastructure.

`domain.ParseEdition()` returns the compile-time edition. See `apps/strait/internal/domain/edition_community.go` and `edition_cloud.go`. Cloud-only files use `//go:build cloud`. Docker: `docker build --build-arg BUILD_TAGS=cloud`.

---

## 6. Local setup

```bash
# 1. Start dependencies
cd apps/strait && docker compose up -d

# 2. Required env (see .env.example for the full list)
export DATABASE_URL=postgres://strait:strait@localhost:15432/strait?sslmode=disable
export REDIS_URL=redis://localhost:16379
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
cd apps/strait && golangci-lint run --timeout=10m ./...
```

When touching DB / queue / workflow / scheduler / pubsub:

```bash
cd apps/strait && go test -tags integration ./...
```

When touching docs (`apps/docs`):

```bash
cd apps/docs && bun run lint
```

This runs the docs linter (`scripts/lint-docs.mjs`): frontmatter completeness, no em/en-dashes, no marketing buzzwords, every code fence has a language tag, internal links and anchors resolve, normalized example hosts, and no orphan pages. CI enforces it through `.github/workflows/docs.yml`.

Lefthook is mandatory:

```bash
lefthook install
```

Hook groups (`lefthook.yml`):
- `pre-commit` (parallel): gitleaks (secret scan of staged files) and Biome (format and check `*.{ts,tsx,js,jsx,mjs,cjs,json,jsonc,css}`).
- `pre-push` (parallel, every branch): gitleaks (secret scan of `origin/master..HEAD`), `manypkg:check`, `biome check .`, `typecheck`, `buf lint` (when `apps/strait/proto/**` changes), and zizmor (GitHub Actions security, when workflows or actions change).
- `commit-msg`: enforces Conventional Commits.

The hooks do not build, test, or lint the Go code. Run those yourself with the validation commands above (golangci-lint is also available as `bun run go:lint`), and CI enforces them on every PR (`test.yml`, `test-race.yml`, `lint.yml`, and others).

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
- **Detailed and complete**: covers the full implementation end to end, not just the first slice.
- **Separated into phases**: each phase is a coherent, independently verifiable unit of work.
- **Presented inline in the conversation**: never write plan files (`PLAN.md`, design docs, and the like) unless the user explicitly asks for one.

For every phase, specify: goal, files to change or create, tests to add or update, and validation commands.

### 8.3 Per-phase execution loop

Once the plan is approved, execute each phase in order. For every phase:

1. Implement the phase exactly as planned.
2. Run the full validation suite:
   ```bash
   cd apps/strait && go build ./...
   cd apps/strait && go test ./...
   cd apps/strait && go test -race ./...
   cd apps/strait && golangci-lint run --timeout=10m ./...
   ```
   Add `cd apps/strait && go test -tags integration ./...` when DB / queue / workflow / scheduler / pubsub behavior is touched.
3. If a check fails:
   - Fix small, local issues immediately and re-run until everything is green.
   - **If the failure requires significantly more work than the phase contemplated** (architectural change, schema rework, cross-cutting refactor, or a newly ambiguous requirement), stop, report the situation to the user, and wait for direction. Do not push through.
4. Once **all** checks pass, commit the phase's changes with a Conventional Commit message (see section 10). Never use `--no-verify` or bypass lefthook.
5. Proceed to the next phase **without asking permission**. Implement the entire plan to completion in one go.

### 8.4 Plan adherence

- Always follow the plan as agreed. If reality forces a deviation, stop, explain, and wait for the user to confirm before proceeding.
- Never silently expand scope mid-phase.
- Never skip validation or commit gates.
- Tests live in the same phase as the behavior change, never deferred to a later phase.
- Update docs, OpenAPI (`apps/strait/schemas/strait.json`), and `.env.example` in the same phase that introduces the change.

---

## 9. Engineering rules (non-negotiable)

1. **No ORM.** Raw SQL with `pgx/v5` patterns in `internal/store`.
2. **Structured concurrency.** Use `sourcegraph/conc` and `alitto/pond/v2`. No casual goroutine fan-out.
3. **Errors.** Wrap with `%w` and contextual messages. Use `samber/oops` for stack traces in critical paths.
4. **Helpers.** Prefer `samber/lo` where it improves readability.
5. **Go style baseline.** Follow the Uber Go Style Guide for new and touched Go code unless it conflicts with this guide or established local patterns. In practice: copy slices/maps at ownership boundaries, avoid pointers to interfaces, verify interface compliance for important adapters, keep contexts first, handle errors once, handle type assertion failures, avoid hidden goroutines, prefer explicit returns, use field names in struct literals, keep zero-value mutexes as values, and justify every `nolint`.
6. **Tests.** Use `apps/strait/internal/testutil` helpers. Use `stretchr/testify/require` for setup and precondition checks that must stop the test, and `stretchr/testify/assert` for independent scalar, boolean, error, containment, and eventually-style checks. Keep `go-cmp`/`testutil.AssertEqual` for complex structs, slices, maps, and option-heavy comparisons where a structural diff is clearer. Write meaningful assertions, not just "no error".
7. **Worker and pool consistency.** Reuse existing patterns in `internal/worker`. Don't fork dispatch logic.
8. **No emojis.** Not in code, comments, logs, docs, commits, or PR text. Anywhere.
9. **Comments.** Explain invariants, security or operational boundaries, and non-obvious tradeoffs. Do not leave phase/wave/project-plan history, AI/tool attribution, or comments that restate the next line. Test comments should name the regression contract, not implementation chronology.
10. **Monitoring is load-bearing.** Preserve traces, metrics, and logs in critical paths (worker, queue, workflow, scheduler).
11. **Auth boundaries.** Don't weaken SSRF guards, the internal management secret flow, or the SDK JWT run-token flow.
12. **Graceful shutdown.** Don't alter shutdown behavior in ways that risk in-flight job loss.

---

## 10. Commits and PRs

### Conventional Commits (mandatory)

Format: `type(scope): summary`

Allowed types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`, `revert`, `style`. Use lowercase types, an imperative summary, and a scope when it helps.

Examples:
- `feat(worker): add retry jitter cap for webhook dispatch`
- `fix(queue): prevent dequeue race on stale heartbeat`
- `test(workflow): add regression for fan-in completion`

`commitlint` runs on every PR (`.github/workflows/commitlint.yml`) against `.commitlintrc.json`, and the lefthook `commit-msg` hook enforces the same type list locally. Commits that don't conform fail the check.

#### Breaking changes

Mark the type with `!` AND include a `BREAKING CHANGE:` footer separated from the body by a blank line:

```text
feat(api)!: drop legacy /v0/jobs endpoint

Migration: callers must move to /v1/jobs. The legacy handler logged
deprecation warnings since v0.0.9.

BREAKING CHANGE: /v0/jobs no longer exists; clients pinned to it
will receive a 404.
```

release-please surfaces `BREAKING CHANGE:` footers in their own section at the top of the release entry and triggers a major bump (a minor bump while the project is pre-1.0).

#### User-facing release notes (commit body)

For PRs whose subject line is not already a clear, one-line user-facing summary, add one to three sentences of prose to the squash commit body. release-please includes the body under the changelog entry, so this is what users read on the release page. The PR template (`.github/pull_request_template.md`) has a "Release notes" section that lands in the squash commit body when GitHub squash-merges.

Skip for: refactors, internal infrastructure, test-only changes, and dependency bumps. Required for: `feat`, `fix`, `perf`, and breaking changes.

**Never** add:
- "Co-Authored-By" lines
- "Generated with Claude Code" or any AI attribution
- Vague messages (`update`, `misc`, `fix stuff`)

### PR descriptions

Substantive, not boilerplate. Include:
- **Summary**: what the PR does in plain language
- **Why**: context and motivation
- **What changed**: grouped by area
- **Validation**: exact commands run and their outcomes
- **Tests added or updated** and why
- **Docs / OpenAPI / env impact** (or an explicit "none")
- **Risks and follow-ups**

Never claim validation without listing the commands. Never paste generic boilerplate that could apply to any PR. Never add AI attribution footers.

### Releases

Releases are fully driven by [release-please](https://github.com/googleapis/release-please) off the conventional commit history on `master`. There is no local `goreleaser`, no manual tag, and no manual changelog edit.

Flow:

1. Land conventional commits on `master`. The `Release Please` workflow runs on every push and keeps a single open release PR up to date with the next version, the rendered `CHANGELOG.md`, and an updated `.release-please-manifest.json`.
2. Merging that PR creates the `vX.Y.Z` git tag and a GitHub Release.
3. The tag push triggers `Publish Docker Images`, which builds, scans (Trivy), signs (cosign keyless), and publishes the community and cloud images plus the strait-app image to GHCR.

Bump rules (release-please reads commit types and `release-please-config.json`):
- `feat:` triggers a minor bump. It stays a minor bump even while pre-1.0, because `bump-patch-for-minor-pre-major` is false.
- `fix:` and `perf:` trigger a patch bump.
- `revert:` triggers a patch bump and appears in the changelog.
- `feat!:` or a `BREAKING CHANGE:` footer triggers a major bump once the project is past 1.0, and a minor bump while pre-1.0 (`bump-minor-pre-major` is true).
- `docs:`, `test:`, `refactor:`, `build:`, `ci:`, `chore:`, and `style:` do not bump the version and are hidden from the changelog.

Version source of truth is `.release-please-manifest.json`. Do not edit it by hand.

#### Tag protection

`v*` tags are protected by a repository ruleset (`Settings -> Rules -> Rulesets -> Protect v* tags`):

- Creation, update, and deletion of any `refs/tags/v*` are restricted.
- Bypass: the `strait-release-please` GitHub App (which release-please uses to push the release tag) and repo admins.

If you need to push or delete a `v*` tag manually, do it as a repo admin or temporarily disable the ruleset. Don't bypass it casually: accidental local tag pushes were the original motivation.

To recreate the ruleset if it's deleted (replace the App ID if regenerated):

```bash
gh api repos/strait-dev/strait/rulesets --method POST --input - <<'JSON'
{
  "name": "Protect v* tags",
  "target": "tag",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["refs/tags/v*"], "exclude": [] } },
  "rules": [
    { "type": "creation" },
    { "type": "update" },
    { "type": "deletion" }
  ],
  "bypass_actors": [
    { "actor_id": 3666235, "actor_type": "Integration", "bypass_mode": "always" },
    { "actor_id": 5,       "actor_type": "RepositoryRole", "bypass_mode": "always" }
  ]
}
JSON
```

---

## 11. DOs and DON'Ts

### Do
- Confirm assumptions when requirements are ambiguous.
- Follow existing package boundaries and naming patterns.
- Keep changes small, focused, and reversible.
- Add tests for new behavior, and regression tests for bug fixes.
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
4. Docs, OpenAPI, and `.env.example` are updated for behavior changes.
5. Migration rules are followed (paired up/down, never edit history).
6. A summary is provided: what changed, why, and how it was validated.

---

When in doubt, prefer established project patterns over novelty, ask clarifying questions early, and keep changes explicit and verifiable.
