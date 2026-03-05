# Core Engine Implementation Plan

Branch: `feat/core-engine`

This plan is execution-ready and reflects all product decisions made in this thread.
It is ordered by risk and dependency, with mandatory quality gates between phases.

## Locked Product Decisions

- Concurrency cap counts `dequeued + executing` runs.
- Project quota overage is rejected at trigger time.
- Execution windows support project-level timezone with optional job-level override.
- If a run is outside an execution window, it remains delayed.
- Queue partitioning supports both project and tag partitions.
- Workers consume multiple partitions via allowlist + weighted polling.
- Per-job rate limiting default is disabled unless configured.
- Dedup uses full canonicalized payload hash and returns existing run.
- Checkpointing is SDK-driven plus auto-snapshots.
- Continuation is lineage-based and parent waits for all descendants terminal.
- Costing uses static pricing table initially.
- Budget overage uses graceful stop behavior.
- Structured outputs require per-output schema validation.
- Tool call tracking stores full tool IO.
- Fallbacks only for transient/rate-limited errors.
- Circuit breaker scope is full endpoint URL.
- Custom retry delay arrays are supported.
- Bulkhead category source: `job.bulkhead_key` > tags > endpoint domain.
- DLQ replay is exposed via API.
- Timeout defaults: connect `3s`, headers `10s`, body = remaining job timeout (min `15s`).
- Tags are string maps.
- Schema transforms are forward-only.
- Payload transforms support JSONPath and JMESPath.
- Secret injection supports headers + payload templates + env-style vars.
- Initial environment is `dev` with inheritance support.
- Replay copies idempotency key.
- Default retention: completed/failed/canceled/expired `30d`; timed_out/crashed/system_failed `90d`.
- SLA tracking is deferred.
- Artifact storage is deferred.

## Non-Negotiable Delivery Rules

Each phase must satisfy all checks before the next phase starts:

1. `golangci-lint run ./...`
2. `go test ./...`
3. `go test -tags integration -race ./internal/store/... ./internal/queue/...`
4. `go test -tags integration -race ./internal/e2e/...`
5. `go build ./...`
6. Phase changes are committed before beginning next phase.

No phase progression is allowed if any gate fails.

---

## Phase Roadmap Summary

- Phase 0: platform primitives and rollout safety.
- Phase 1: safe high-value behavior + additive schema.
- Phase 2: execution model and scheduling controls.
- Phase 3: AI long-running and budget engine.
- Phase 4: resilience hardening + developer ergonomics.
- Phase 5: low-priority/deferred backlog.

---

## Phase 0 - Primitives and Safety

### Goals

- Add rollout control, atomic transaction support, and benchmark baseline before queue hot-path changes.

### Implementation Backlog

#### Config and feature flags

- Files:
  - `internal/config/config.go`
  - `internal/config/config_test.go`
- Add typed flags (booleans) for all major upcoming features, grouped by area:
  - execution model (`FF_CONCURRENCY_LIMITS`, `FF_PROJECT_QUOTAS`, ...)
  - AI support (`FF_CHECKPOINTS`, `FF_USAGE_TRACKING`, ...)
  - resilience (`FF_CIRCUIT_BREAKER`, `FF_SMART_RETRY`, ...)
  - ergonomics (`FF_TAGS`, `FF_DRY_RUN`, ...)
- Add tests for default values and env parsing.

#### Store transaction helper

- Files:
  - `internal/store/store.go`
  - `internal/store/store_integration_test.go`
- Add `WithTx(ctx, pool, fn)` helper.
- Ensure rollback/commit behavior is verified in integration tests.

#### Maintenance loop abstraction

- New files:
  - `internal/scheduler/maintenance.go`
  - `internal/scheduler/maintenance_test.go`
- Provide generic ticker loop with context cancellation, structured logging, and failure backoff.
- Rewire reaper to use this abstraction (no behavior change).

#### SDK capability/versioning

- Files:
  - `internal/api/sdk.go`
  - `internal/api/server.go`
  - `internal/api/sdk_test.go`
- Add `X-SDK-Version` capture and capability registry.
- Backward-compatible: missing header defaults to legacy behavior.

#### Baseline performance harness

- Files:
  - `internal/queue/queue_integration_test.go`
  - `internal/worker/executor_test.go`
- Add benchmark suite for dequeue throughput, lock contention, and p95 dequeue latency.

### Phase 0 Exit Criteria

- Feature flags parse and tests pass.
- `WithTx` exists and is used in at least one path.
- Reaper runs via maintenance abstraction with no regressions.
- SDK version is accepted and test-covered.
- Baseline benchmark report checked into repo notes.

---

## Phase 1 - Safe Core Upgrades + Additive Schema

### Features in this phase

- `2.21` Error classification
- `2.28` Payload validation enforcement
- `2.9` Streaming progress
- `2.35` Run retention
- `2.40` Execution tracing
- Schema foundations for later phases

### Migration Plan (additive only)

Proposed migration sequence:

1. `000014_add_engine_control_columns.up.sql`
   - `jobs.max_concurrency INT NULL`
   - `jobs.execution_window_cron TEXT NULL`
   - `jobs.timezone TEXT NULL`
2. `000015_create_project_quotas.up.sql`
3. `000016_add_run_error_and_metadata.up.sql`
   - `job_runs.error_class TEXT NULL`
   - `job_runs.metadata JSONB NOT NULL DEFAULT '{}'`
4. `000017_create_run_checkpoints.up.sql`
5. `000018_create_run_usage.up.sql`
6. `000019_create_pricing_catalog.up.sql`
7. `000020_create_run_tool_calls.up.sql`
8. `000021_create_run_outputs.up.sql`
9. `000022_create_endpoint_circuit_state.up.sql`
10. `000023_create_job_secrets.up.sql`

All `down.sql` files must be provided even if rollback is operationally discouraged.

### API and behavior backlog

#### Error classification (`2.21`)

- Files:
  - `internal/domain/errors.go`
  - `internal/worker/executor.go`
  - `internal/store/runs.go`
- Classify as: `transient`, `client`, `rate_limited`, `server`, `auth`, `unknown`.
- Persist class on terminal/retry transitions.

#### Payload schema enforcement (`2.28`)

- Files:
  - `internal/api/trigger.go`
  - `internal/api/jobs.go`
  - `internal/api/handler_test.go`
- Validate trigger payload against `payload_schema` before enqueue.
- Return deterministic validation errors.

#### Streaming progress (`2.9`)

- Files:
  - `internal/api/sdk.go`
  - `internal/api/stream.go`
  - `internal/store/events.go`
  - `internal/domain/types.go`
- Add SDK progress endpoint supporting:
  - `percent` (0-100)
  - `message`
  - optional `step`
  - optional `eta_seconds`
- Publish through existing SSE pipeline.

#### Run retention (`2.35`)

- Files:
  - `internal/scheduler/reaper.go` or new retention worker under `internal/scheduler/`
  - `internal/store/runs.go`
- Apply defaults:
  - completed/failed/canceled/expired => 30d
  - timed_out/crashed/system_failed => 90d
  - non-terminal => never auto-delete

#### Execution tracing (`2.40`)

- Files:
  - `internal/worker/executor.go`
  - `internal/telemetry/metrics.go`
  - `internal/store/events.go`
- Record timing breakdown fields:
  - queue_wait
  - dequeue
  - connect
  - ttfb
  - transfer
  - total

### Phase 1 Test Matrix

#### Unit tests

- Error classifier mapping tests.
- Payload validation edge cases.
- Progress payload validation tests.
- Retention policy selector tests.
- Trace duration computation tests.

#### Integration tests

- Trigger with valid/invalid payload schema.
- Progress endpoint writes event + streams via pubsub.
- Retention worker removes eligible runs and keeps protected ones.
- Migration up/down idempotency checks.

#### E2E tests

- Trigger -> progress updates -> completion -> SSE verification.
- Failure path stores `error_class` and remains queryable.

### Phase 1 Exit Criteria

- All migrations apply cleanly.
- Error class populated for new failures.
- Payload validation enforced.
- Progress visible via SSE and persisted.
- Retention worker active and tested.
- Trace metrics emitted and observable.

---

## Phase 2 - Execution Model and Scheduling Controls

### Features

- `2.1`, `2.2`, `2.3`, `2.4`, `2.5`, `2.7`, `2.8`

### Notes

- Keep enforcement behind flags initially.
- Protect queue hot path by starting with conservative claim + guard, then optimize.

---

## Phase 3 - AI Long-Running Support

### Features

- `2.10`, `2.11`, `2.12`, `2.13`, `2.14`, `2.15`, `2.16`, `2.17`, `2.18`

### Notes

- Parent run waits until all descendants terminal for continuation.
- Budget governor uses graceful stop and checkpoint-aware transitions.

---

## Phase 4 - Reliability and Ergonomics

### Features

- Reliability: `2.19`, `2.20`, `2.22`, `2.23`, `2.24`
- Ergonomics/data/obs: `2.25`, `2.26`, `2.27`, `2.29`, `2.30`, `2.31`, `2.32`, `2.33`, `2.34`, `2.37`, `2.38`, `2.39`, `2.41`, `2.45`

---

## Phase 5 - Deferred

- `2.36` Run artifact storage (explicitly deferred)
- `2.42` SLA tracking (deferred)
- Optional low-priority backlog: `2.6`, `2.43`, `2.44`

---

## Commit Strategy Per Phase

Use atomic commits in this order:

1. migrations
2. domain/store
3. api
4. worker/scheduler/queue
5. tests
6. docs

No cross-phase mixing in a single commit batch.

---

## Immediate Next Execution Slice

Start Phase 0 in this exact sequence:

1. feature flags
2. `WithTx`
3. maintenance loop abstraction
4. SDK versioning
5. baseline benchmarks
6. full lint + tests + build
7. commit Phase 0

Then execute Phase 1 in the same gated pattern.
