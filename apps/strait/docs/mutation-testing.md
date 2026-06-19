# Mutation Testing

Strait uses Gremlins for Go mutation testing. Run it package by package. Do
not run Gremlins across the whole module: the module is large enough that a
full run can exhaust memory or take hours.

## Local workflow

Start with a dry run to discover mutants:

```bash
cd apps/strait
GREMLINS_DRY_RUN=1 ./scripts/mutation-test.sh ./internal/errors
```

Run mutation testing for the same package:

```bash
cd apps/strait
./scripts/mutation-test.sh ./internal/errors
```

The same command is available through Make:

```bash
cd apps/strait
make mutation-test PKG=./internal/errors
```

The wrapper pins Gremlins to `v0.6.0` by default, uses one worker, limits each
`go test` process to one CPU, uses a timeout coefficient of 60, and writes JSON
results under `.cache/gremlins`.
It refuses package patterns containing `...` so mutation work stays explicitly
package scoped.

Useful overrides:

```bash
GREMLINS_WORKERS=2 ./scripts/mutation-test.sh ./internal/errors
GREMLINS_OUTPUT_STATUSES=lctv ./scripts/mutation-test.sh ./internal/errors
GREMLINS_TAGS=cloud ./scripts/mutation-test.sh ./internal/billing
GREMLINS_OUTPUT_LABEL=route \
  GREMLINS_EXCLUDE_FILES='pgque_dequeue.go,pgque_ready.go' \
  ./scripts/mutation-test.sh ./internal/queue
```

Keep `GREMLINS_WORKERS` and `GREMLINS_TEST_CPU` low. If a package times out
because its baseline tests are very fast relative to Go startup and compile
time, raise `GREMLINS_TIMEOUT_COEFFICIENT` for that package before treating the
timeouts as real findings.
For broad packages, use `GREMLINS_EXCLUDE_FILES` with filepath regexes to run a
bounded file slice while keeping the package argument explicit. Set
`GREMLINS_OUTPUT_LABEL` for every slice so the JSON result does not overwrite
the package-level result.

## Package rollout

For each package:

1. Run a dry run and note `NOT COVERED` mutants.
2. Run the real mutation test and inspect `LIVED`, `TIMED OUT`, and `NOT VIABLE`
   results.
3. Add or tighten package tests for meaningful surviving mutants.
4. Re-run the same package until the remaining survivors are documented as
   equivalent, non-viable, or intentionally out of scope.
5. Commit the package's test improvements separately from unrelated refactors.

The first gate is advisory. Do not add a blocking CI threshold for a package
until that package has an explicit target and stable runtime.

## In-progress packages

These packages are still active rollout targets, or have not yet completed a
full mutation run with the default bounded settings:

| Package | Dry-run runnable mutants | Not covered | Notes |
| --- | ---: | ---: | --- |
| `./cmd/strait` | 0 | 382 | Entrypoint wiring is not covered by current tests; needs command/service harness work before full mutation testing is useful. |
| `./internal/api/grpc` | 377 | 149 | Broad worker-plane surface; auth, auth cache, replica ID, registry, recovery/logging interceptors, Sentry interceptor, worker metrics, server startup/TLS, dispatch helpers, sweep, and stream control-loop helper slices are covered with 368 killed mutants and one intentional forced-stop timeout gap. Sweep has no lived mutants with 30 killed and 2 constant-only not covered after covering stale-task recovery, stale-worker eviction, offline deletion, durable result handoff, finalizer retry, and recovered-run requeue paths. Remaining dispatch/stream gaps are mostly DB-backed worker-task, audit, and end-to-end stream paths. |
| `./internal/billing` | 1060 | 453 | Large default-edition surface; retention resolver, entitlement snapshot, project cost aggregation, plan-limit resolution, catalog resolver, PostHog client, Prometheus uptime source, threshold warning, add-on limit, billing email sender, welcome email, webhook dispatcher, downgrade preview, and SLA credit slices are clean with 258 killed mutants. Split by enforcement, webhook, usage, and email subareas before a full run. |
| `./internal/loadtest` | 295 | 638 | Broad load-test harness surface; untagged runtime profile, audit emit harness, queue bloat gate, queue benchmark report, and performance baseline report helper slices are clean with 295 killed mutants. Remaining gaps are mostly build-tagged reporting, scenarios, and server helpers. |
| `./internal/queue` | 782 | 0 | Full package dry run has 100% mutator coverage. Enqueue terminal error helper, backpressure, enqueue retry, PgQue route selection, PgQue claim helper, PgQue dequeue, PgQue ready, PgQue enqueue, PgQue runtime, health sampler, notify, run writer, PgQue core defaults, and queue metrics slices are clean with 702 killed mutants. Full real package run hit temp disk pressure; keep using bounded real slices. |
| `./internal/scheduler` | 1173 | 65 | Broad scheduler surface; budget monitor, scheduler metrics, counter reconciler, partition tuner, outbox archiver, ready-run reconciler, debounce run, plan drift monitor, grace period enforcer, partition reclaimer, partition ensurer, priority promoter, DLQ age-out, heartbeat GC, idempotency GC, delayed poller, memory cleanup, event type matching, maintenance loop, webhook message cleanup, concurrent reconciler, usage report candidate, backpressure sampler, anomaly monitor, core scheduler reloader, SLO webhook adapter, stale subscription checker, advisory lock, recovery/shutdown helpers, component registration, quota resume advisory-lock, backpressure token cap, cron, usage report and forecast emailers, usage flusher, batch flusher, contract expiry, watched query, stats aggregator, and index maintenance slices are clean with 735 killed mutants; audit reaper has no lived mutants with 72 killed and 1 not covered after covering audit retention metric guards and previously-reclaimed delete failures; debounce poller has no lived mutants with 47 killed and 2 not covered after covering non-transactional admission plus active-run and daily-cost quotas; downgrade applier has full dry coverage and no lived mutants with 58 killed after covering HTTP-mode downgrade pause and dispatch paths; SLO evaluator has full dry coverage and no lived mutants with 42 killed after covering run-loop defaults and cycle error logging; reaper dry coverage is down to 5 constant-only uncovered mutants after covering workflow callback errors, fallback advisory-lock errors, approval-reminder channel lookup failures, paused workflow timeout fallback, rotation webhook guard/default-client paths, and history-retention error reporting; outbox flusher has no lived mutants with 25 killed and 57 not covered after covering constructor defaults, accessors, panic recovery, and outbox row mapping. Continue splitting DB-backed reapers, monitors, and reconciliation helpers before a full run. |
| `./internal/store` | 402 | 2155 | Store package has many DB accessors with little direct self-coverage; target stable store areas with integration-backed tests before a full run. |
| `./internal/testutil` | 21 | 245 | Test helper package has little direct self-coverage; existing testcrypto, pgxslow, and cmp helper slices are clean with 19 killed mutants, and the seed-pentest validation slice has no lived mutants with 3 killed and 16 not covered. Remaining gaps are mostly DB, Redis, factory, and assertion helper setup paths. |
| `./internal/worker` | 997 | 0 | Full package dry run has 100% mutator coverage. Bounded real slices reconcile to all 997 runnable mutants killed across executor, runtime, resilience, completion, subscriber, webhook, metrics, heartbeat, pool, cache, signing, validation, and DLQ surfaces. Keep using exact-path excludes for overlapping names such as `metrics.go` and `subscriber_metrics.go`; a full one-worker real run is unnecessary for coverage and has higher disk pressure risk. |

## Current clean packages

These packages have been run through `./scripts/mutation-test.sh` with the
default bounded settings and reached 100% efficacy and 100% mutant coverage:

| Package | Mutants killed | Notes |
| --- | ---: | --- |
| `./internal/errors` | 2 | Harness smoke package |
| `./internal/debug` | 7 | No test changes required |
| `./internal/dbscan` | 28 | No test changes required |
| `./internal/apikeycache` | 10 | Added refresh and loader error coverage |
| `./internal/eventfilter` | 28 | Added size-limit coverage; removed const arithmetic noise |
| `./internal/migrationlint` | 64 | Removed unreachable sort tie-break |
| `./internal/bundle` | 14 | No test changes required |
| `./internal/health` | 25 | Timeout coefficient 60 avoids false timeout noise |
| `./internal/crypto` | 53 | Added field/HMAC coverage; removed helper type arithmetic noise |
| `./internal/ratelimit` | 55 | No test changes required |
| `./internal/notification` | 79 | Removed lease-duration const arithmetic noise |
| `./internal/httputil` | 87 | Added dialer DNS selection and transport/sanitizer coverage |
| `./internal/config` | 175 | Added production unset-sslmode regression; simplified SSL-mode branch shape |
| `./internal/logdrain` | 125 | Added test-client guard coverage; removed duration arithmetic noise |
| `./internal/pubsub` | 45 | Added real Redis publisher unit coverage; simplified subscribe cleanup |
| `./internal/domain` | 81 | Added lifetime, region, clone, and timeout-bound coverage |
| `./internal/webhook` | 271 | Added cost-recorder and tenant-scope coverage; slow full run (~15m30s) |
| `./internal/testutil/pgxslow` | 3 | No test changes required |
| `./internal/workflow/testing` | 24 | No test changes required |
| `./internal/cdc` | 359 | Added CDC edge coverage; slow full run (~21m47s) |
| `./internal/clickhouse` | 241 | Added scripted SQL success-path coverage for analytics scans, schema creation, and buffer helpers |
| `./internal/telemetry` | 263 | Added watchdog, pool sampler, Redis, and Sentry edge coverage; slow full run (~10m02s) |
| `./internal/cache` | 301 | Added cache bus, read-model, registry, Redis L2, and consistency edge coverage; slow full run (~17m30s) |
| `./cmd/gen-audit-schema` | 2 | Added writer-injected command harness coverage |
| `./scripts/dump-openapi` | 5 | Added injected command runner coverage for output, random, fetch, and write paths |
| `./scripts/format-benchmarks` | 40 | Added parser, runner, markdown, aggregate, and formatting coverage |
| `./test/loadtest/cmd/summary` | 35 | Added parser, renderer, command, and throughput summary coverage |
| `./scripts/check-openapi-parity` | 39 | Added command, comparison, parser, and path-normalization coverage |
| `./internal/workflow` | 1255 | Added raw condition, JSON scanner, object payload merge, step override filtering, expected-completion, simulator conditional, workflow run helper, topological helper, progression fallback, bootstrap fallback, root max-parallel, cost-gate, retry transaction, payload scanner, and callback progression coverage; slow full run (~1h31m) |
