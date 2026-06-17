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
```

Keep `GREMLINS_WORKERS` and `GREMLINS_TEST_CPU` low. If a package times out
because its baseline tests are very fast relative to Go startup and compile
time, raise `GREMLINS_TIMEOUT_COEFFICIENT` for that package before treating the
timeouts as real findings.

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
