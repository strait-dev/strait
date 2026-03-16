# AGENTS.md

This file is the operating guide for contributors and AI agents working on this repository.

**Read this document before making any change.**
If instructions conflict, use this priority order:
1. Direct user request
2. Repository conventions in this file
3. Existing code patterns
4. Personal preference

---

## 1) What this project is

- **Project**: Strait
- **Language**: Go 1.26
- **Module**: `strait`
- **Purpose**: Job execution/orchestration platform with:
  - job definitions and triggering (HTTP dispatch + managed container execution)
  - run lifecycle management (13-state FSM with pause/resume)
  - workflow DAG orchestration (steps, approvals, sub-workflows, wait-for-event)
  - managed execution on Fly Machines (warm pool, pause/resume reuse, cost tracking)
  - SDK endpoints for in-run operations (progress, checkpoint, completion)
  - ClickHouse analytics (optional, for run metrics and cost reporting)
  - webhook delivery with retry, circuit breaker, and HMAC signing
  - observability/metrics/tracing (OpenTelemetry + Prometheus)

Runtime modes:
- `api` (HTTP API)
- `worker` (execution/scheduler)
- `all` (combined)

Core technical model:
- PostgreSQL is the source of truth and queue backend (`SELECT ... FOR UPDATE SKIP LOCKED`)
- Redis supports pub/sub and streaming integrations
- Single binary deployment model

Start here for high-level context:
- `README.md`
- `docs/introduction.mdx`
- `docs/quickstart.mdx`
- `docs/architecture.mdx`

---

## 2) Repository map (how to navigate)

Top-level directories you will use most:

- `apps/strait/cmd/strait/` — CLI commands and app entrypoint wiring
- `apps/strait/internal/api/` — HTTP routes, middleware, API auth paths
- `apps/strait/internal/worker/` — execution worker pool and dispatch behavior
- `apps/strait/internal/workflow/` — DAG orchestration engine and step progression
- `apps/strait/internal/compute/` — container runtime abstraction (Fly Machines, Docker), warm machine pool, machine lifecycle, cost estimation
- `apps/strait/internal/scheduler/` — cron/poller/reaper/retention/pool-pruner background loops
- `apps/strait/internal/queue/` — dequeue/queue logic and concurrency-safe claiming
- `apps/strait/internal/store/` — raw SQL data access layer
- `apps/strait/internal/domain/` — domain models, FSM types/errors
- `apps/strait/internal/clickhouse/` — optional ClickHouse analytics export (run events, analytics, compute usage)
- `apps/strait/internal/webhook/` — async webhook delivery with retry, circuit breaker, HMAC signing
- `apps/strait/internal/cdc/` — change data capture via Sequin for real-time event streaming
- `apps/strait/internal/pubsub/` — Redis-backed pub/sub for SSE streaming and event notifications
- `apps/strait/internal/logdrain/` — external log forwarding (HTTP, Datadog, Splunk)
- `apps/strait/internal/ratelimit/` — per-job and per-API rate limiting
- `apps/strait/internal/crypto/` — secret encryption, API key hashing, signature verification
- `apps/strait/internal/telemetry/` — OpenTelemetry tracing and Prometheus metrics
- `apps/strait/internal/config/` — env var loading/defaults/validation
- `apps/strait/internal/testutil/` — factories/assert helpers/cmp tools
- `apps/strait/migrations/` — SQL migrations (embedded in binary)
- `docs/` — product + dev + API + CLI docs

Useful support files:
- `.github/workflows/lint.yml`
- `.github/workflows/test.yml`
- `.golangci.yml`
- `lefthook.yml`
- `.env.example`
- `docker-compose.yml`

---

## 3) Documentation reading protocol (mandatory)

Before implementation, read docs intentionally instead of guessing.

### 3.1 Use docs navigation as source of truth
- `docs/docs.json` defines official docs structure/tabs/pages.

### 3.2 Read by change type

- **Runtime behavior**:
  - `docs/architecture.mdx`
  - `docs/concepts/runs.mdx`
  - `docs/concepts/workflows.mdx`
  - `docs/concepts/scheduling.mdx`

- **Configuration / env vars**:
  - `apps/strait/internal/config/config.go`
  - `docs/configuration/environment-variables.mdx`
  - `.env.example`

- **Database / queue / store / FSM**:
  - `docs/development/database-schema.mdx`
  - relevant files in `apps/strait/migrations/`
  - `apps/strait/internal/store/*`, `apps/strait/internal/queue/*`

- **CLI changes**:
  - `docs/cli/overview.mdx`
  - relevant `docs/cli/*.mdx`
  - matching files in `apps/strait/cmd/strait/*`

- **Auth / security changes**:
  - `docs/guides/authentication.mdx`
  - `docs/guides/security.mdx`

- **Managed execution / Fly Machines / compute**:
  - `docs/concepts/managed-execution.mdx`
  - `apps/strait/internal/compute/*`
  - `apps/strait/internal/worker/executor_dispatch.go` (managedDispatch function)
  - `apps/strait/internal/worker/executor.go` (pool wiring, pruner, shutdown drain)

- **ClickHouse / analytics**:
  - `docs/concepts/clickhouse-analytics.mdx`
  - `apps/strait/internal/clickhouse/*`

- **Webhooks / event delivery**:
  - `docs/concepts/webhooks.mdx`
  - `docs/concepts/webhook-subscriptions.mdx`
  - `apps/strait/internal/webhook/*`

- **Testing strategy**:
  - `docs/development/testing.mdx`
  - `apps/strait/internal/testutil/*`

- **Public API contract**:
  - `docs/api-reference/overview.mdx`
  - `docs/openapi.yaml`

### 3.3 Clarification rule (mandatory)

**If you have any unresolved questions before implementing a plan, always ask the user and wait for feedback before proceeding.**

Do not continue with assumptions that can change architecture, schema, API behavior, or user-facing semantics.

---

## 4) Engineering rules (non-negotiable)

From project conventions (`docs/development/contributing.mdx`) + existing codebase practice:

1. **No ORM**
   - Use raw SQL with `pgx/v5` patterns in `apps/strait/internal/store`.

2. **Concurrency discipline**
   - Prefer structured concurrency (`sourcegraph/conc` and existing patterns).
   - Do not introduce unmanaged goroutine patterns casually.

3. **Worker/pool consistency**
   - Reuse existing worker execution/pool patterns in `apps/strait/internal/worker`.

4. **Error handling**
   - Wrap with `%w` and include contextual message.

5. **Collection helpers**
   - Prefer `samber/lo` where it improves readability.

6. **Testing style**
   - Use `apps/strait/internal/testutil` helpers, especially structural comparisons.

7. **No emojis**
   - In code, comments, logs, docs, commits, PR text.

---

## 5) Local setup and commands

### 5.1 Start dependencies
```bash
docker compose up -d
```

### 5.2 Minimum required environment
```bash
export DATABASE_URL=postgres://strait:strait@localhost:5432/strait?sslmode=disable
export REDIS_URL=redis://localhost:6379
export INTERNAL_SECRET=<32+ chars>
export JWT_SIGNING_KEY=<32+ chars>
```

### 5.3 Run app
```bash
cd apps/strait && go run ./cmd/strait --mode all
```

References:
- `docs/quickstart.mdx`
- `docs/development/contributing.mdx`
- `docker-compose.yml`
- `.env.example`

---

## 6) Validation commands (before proposing merge)

Run relevant commands for your scope:

```bash
cd apps/strait && go build ./...
cd apps/strait && go test ./...
cd apps/strait && go test -race ./...
cd apps/strait && golangci-lint run --timeout=5m ./...
```

When applicable:

```bash
cd apps/strait && go test -tags integration ./...
cd apps/strait && go test -bench . ./internal/...
```

CI references:
- `.github/workflows/lint.yml`
- `.github/workflows/test.yml`
- `.golangci.yml`

Git hooks:
```bash
lefthook install
```

---

## 7) Database and migration safety

1. Never edit historical migrations already merged.
2. Add new migration pairs only:
   - `NNNNNN_name.up.sql`
   - `NNNNNN_name.down.sql`
3. Create via helper command:
```bash
cd apps/strait && go run ./cmd/strait migrate create <name>
```
4. Validate locally:
```bash
cd apps/strait && go run ./cmd/strait migrate up
cd apps/strait && go run ./cmd/strait migrate status
```

References:
- `docs/development/database-schema.mdx`
- `apps/strait/cmd/strait/migrate.go`

---

## 8) Workflow for implementing changes

1. Understand request and constraints.
2. Read relevant docs and nearest code paths.
3. If ambiguous, ask user and wait.
4. Share a concise implementation plan for non-trivial work.
5. Implement minimal targeted change.
6. Add/update tests (mandatory for new functionality and bug fixes).
7. Update docs/contracts/config examples if needed.
8. Run validations and report results (include exact test commands and outcomes).

Keep scope narrow: one logical change per PR.

---

## 9) DOs and DON'Ts

### DO
- Do confirm assumptions when requirements are ambiguous.
- Do follow existing package boundaries and naming patterns.
- Do keep changes small, focused, and reversible.
- Do add tests for every new functionality.
- Do include regression tests for bug fixes.
- Do expand coverage when touching critical paths (worker, queue, workflow, scheduler, store).
- Do maintain backward compatibility unless user requests breakage.
- Do update:
  - CLI docs for CLI behavior changes
  - OpenAPI/docs for API shape changes
  - env docs + `.env.example` for config changes
- Do preserve observability (logs/metrics/traces) in critical paths.

### DON'T
- Don’t guess business rules, API contracts, or schema intent.
- Don’t make unrelated refactors in the same PR.
- Don’t ship new behavior without tests.
- Don’t bypass failing tests/lint without explicit user approval.
- Don’t weaken auth/security/SSRF protections.
- Don’t alter shutdown behavior in ways that risk in-flight job loss.
- Don’t mark work as complete without validation evidence.

---

## 10) Commit and PR conventions

### 10.1 Conventional Commits (mandatory)

Every commit must follow Conventional Commits:

```text
type(scope): short summary
```

Examples:
- `feat(worker): add retry jitter cap for webhook dispatch`
- `fix(queue): prevent dequeue race on stale heartbeat`
- `docs(cli): clarify runs watch output modes`
- `test(workflow): add regression for fan-in completion`

Allowed types:
- `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`, `revert`

Rules:
1. lowercase type/scope
2. imperative summary
3. include scope when useful
4. use `!` for breaking changes and explain in body
5. avoid vague messages (`update`, `misc`, `fix stuff`)

### 10.2 PR expectations (quality bar)

PR descriptions must be clear, complete, and easy to review. Avoid short/vague descriptions.

Every PR description should include:
- **Summary**: what this PR does in plain language
- **Why**: context/problem and motivation
- **What changed**: key implementation points (grouped by area)
- **Validation**: exact commands run + outcomes
- **Testing impact**: what tests were added/updated and why
- **Docs/contract impact**: OpenAPI/CLI/docs/env changes (or explicit “none”)
- **Risk & rollout notes**: risks, mitigations, follow-ups

### 10.2.1 PR description template (recommended)

```md
## Summary

## Why

## What Changed
- 
- 

## Validation
- Commands run:
  - `cd apps/strait && go test ./...`
  - `cd apps/strait && go test -race ./...`
  - `cd apps/strait && golangci-lint run --timeout=5m ./...`
- Result summary:

## Testing Impact
- New tests:
- Updated tests:
- Regression coverage:

## Docs / Contract Impact
- Docs updated:
- OpenAPI updated:
- Env/config updates:

## Risks / Follow-ups
- Risks:
- Follow-up work:
```

### 10.2.2 PR description DON'Ts

- Don’t omit **why** the change is needed.
- Don’t paste generic text that could apply to any PR.
- Don’t claim validation without listing commands/results.
- Don’t skip test impact details for behavior changes.

### 10.3 PR testing notes template (mandatory)

Use this section in every PR:

```md
## Testing Notes

### Scope
- Changed areas:
- Risk level: low | medium | high

### Tests Added/Updated
- [ ] Unit tests
- [ ] Integration tests
- [ ] Regression tests
- [ ] Race-sensitive tests

Files:
- `path/to/test_file_1.go`
- `path/to/test_file_2.go`

### Commands Run
- `cd apps/strait && go test ./...`
- `cd apps/strait && go test -race ./...`
- `cd apps/strait && go test -tags integration ./...` (if applicable)
- `cd apps/strait && golangci-lint run --timeout=5m ./...`

### Results
- Summary:
- Any failing/skipped tests and why:
- Follow-up test debt (if any):
```

If you skip any relevant test category, explicitly justify it in the PR.

---

## 11) Consistency checklists

### 11.1 API / CLI / docs consistency

- [ ] API request/response shapes match `docs/openapi.yaml`
- [ ] CLI flags/output documented in `docs/cli/*.mdx`
- [ ] New env vars wired in `apps/strait/internal/config/config.go`
- [ ] Env docs + `.env.example` updated
- [ ] Feature flag behavior documented where relevant

### 11.2 Testing expectations by change type

- Domain logic -> table-driven unit tests
- Concurrency/worker/scheduler -> race-aware tests
- Store/query/queue changes -> integration coverage preferred
- Workflow logic -> DAG/edge-case progression tests
- Bug fix -> regression test
- New functionality -> at least one happy-path test + relevant failure/edge-case tests

Use `apps/strait/internal/testutil/*` helpers whenever possible.

### 11.3 Testing quality bar (mandatory)

We prioritize correctness and confidence over speed of merging.

- Every behavior change should be protected by tests.
- New functionality should include meaningful assertions (not only "no error").
- For critical execution paths, prefer multiple tests covering:
  - success path
  - validation/error path
  - retry/timeout behavior (when applicable)
  - concurrency/race risks (when applicable)
- When fixing defects, add a regression test that would fail before the fix.
- If a test is hard to write, treat that as a design signal and improve seams/interfaces.

Minimum validation expectation before handoff:

```bash
cd apps/strait && go test ./...
cd apps/strait && go test -race ./...
```

And when touching DB/queue/workflow/scheduler behavior:

```bash
cd apps/strait && go test -tags integration ./...
```

---

## 12) Security and operations guardrails

- Never commit credentials/secrets/tokens.
- Preserve auth model boundaries:
  - internal management API secret flow
  - SDK JWT run-token flow
- Keep SSRF and endpoint validation safeguards intact.
- Preserve graceful shutdown + in-flight job safety.
- Be explicit when changing retries/timeouts/dead-letter behavior.

References:
- `docs/guides/security.mdx`
- `docs/guides/authentication.mdx`
- `docs/architecture.mdx`

---

## 13) Definition of done

A change is done only when all apply:

1. Code compiles (`cd apps/strait && go build ./...`)
2. Relevant tests pass (including race/integration when needed)
3. Lint passes
4. Docs/contracts/config updated for behavior changes
5. Migration rules followed (if schema changed)
6. Summary provided: what changed, why, and how validated

---

## 14) High-value reference index

- `README.md`
- `docs/architecture.mdx`
- `docs/quickstart.mdx`
- `docs/concepts/managed-execution.mdx`
- `docs/concepts/clickhouse-analytics.mdx`
- `docs/concepts/workflows.mdx`
- `docs/concepts/runs.mdx`
- `docs/development/contributing.mdx`
- `docs/development/testing.mdx`
- `docs/development/database-schema.mdx`
- `docs/configuration/environment-variables.mdx`
- `docs/openapi.yaml`
- `docs/cli/`
- `.github/workflows/lint.yml`
- `.github/workflows/test.yml`
- `.golangci.yml`
- `lefthook.yml`

---

When in doubt, prefer established project patterns over novelty, ask clarifying questions early, and keep changes explicit and verifiable.
