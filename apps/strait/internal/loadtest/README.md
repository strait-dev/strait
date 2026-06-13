# Load Testing Framework

Measure Strait's performance limits. Tests progressively increase load to find throughput ceilings, concurrency limits, and stability issues.

## Source Of Truth

| Area | Source |
|---|---|
| Make targets | `apps/strait/Makefile` |
| Test entrypoints | `loadtest_test.go` |
| Scenarios | `scenarios.go` |
| Report generation | `report.go` |
| Results directory | `loadtest-results/` |

## Quick Start

From `apps/strait/`:

```bash
# 1. Start Postgres and Redis (see apps/strait/ docker compose up -d)
make dev-up

# 2. Start Strait configured for load testing
make loadtest-server

# 3. Run the 15-minute quick validation
make loadtest-quick
```

That's it. The quick validation ramps from 10 to ~100 jobs/sec and finds your
approximate throughput ceiling.

Start with `loadtest-quick` to find your approximate ceiling. If you need precise
numbers, run `loadtest-throughput` and `loadtest-concurrency`. For production
validation, run `loadtest-endurance`.

## All Commands

```bash
make loadtest-server       # Start Strait configured for load testing
make loadtest-quick        # 15-min quick validation (CI-friendly)
make loadtest-throughput   # Find max sustained throughput (up to 2h)
make loadtest-concurrency  # Find max concurrent connections (up to 1h)
make loadtest-endurance    # 24h stability test at 70% ceiling
make loadtest-chaos        # Run all chaos engineering scenarios
make loadtest-errors       # Test all 12 error scenarios
make loadtest-all          # Run quick + throughput + concurrency
make loadtest-report       # Generate HTML/JSON report from results
make loadtest-unit         # Run unit tests for the framework itself
```

Note: The `make loadtest-*` targets are defined in the loadtest Makefile at `apps/strait/Makefile`. Run them from the `apps/strait/` directory.

## Test Tiers

| Tier | Test | Duration | What It Finds |
|------|------|----------|---------------|
| 0 | Quick Validation | 15 min | Approximate throughput ceiling |
| 1 | Throughput Ceiling | ~1-2h | Exact max jobs/sec before breaking |
| 2 | Concurrency Ceiling | ~1h | Max concurrent connections |
| 3a | Multi-Tenant Simulation | 4-8h | Real production behavior with 500-2000 tenants |
| 3b | Breaking Point | 2-6h | Exact tenant count where system degrades |
| 4 | Endurance | 24-72h | Memory leaks, goroutine leaks, performance drift |
| 5 | Chaos Engineering | ~4h | Recovery from failure scenarios |

## Monitoring Pipeline

Strait exports telemetry through several channels:

| Signal | Destination | Notes |
|--------|------------|-------|
| Metrics (Prometheus) | Grafana Cloud via OTel collector | 45+ custom metrics, scraped every 5s |
| Traces (OTLP) | Grafana Cloud / ClickHouse | Spans include `deployment.environment` |
| Structured logs (OTLP) | ClickHouse via OTel collector | slog records bridged through `otelslog` |
| Error tracking | Sentry | Environment-aware, PII-scrubbed |
| Customer run events | ClickHouse | Batch-exported via async exporter |
| Log drains | Customer webhooks | Worker dispatches events to configured endpoints |

All OTel resources include `service.name` and `deployment.environment` attributes
so metrics, traces, and logs can be correlated and filtered by environment.

## Architecture

```text
  loadtest_test.go          Test entrypoints (TestQuickValidation, etc.)
       │
       ├── harness.go       Infrastructure: DB pool, Redis, HTTP server, metrics
       ├── ramp.go          Throughput and concurrency ramp engines
       ├── scenarios.go     Pre-configured test scenarios per tier
       ├── tenant_simulator Multi-tenant traffic with Poisson + time-of-day
       ├── endurance.go     24-72h stability with spike injection + leak detection
       ├── chaos.go         8 chaos scenarios (worker kill, DB failover, etc.)
       ├── metrics.go       Go runtime, Postgres pool, Redis stats (JSONL output)
       ├── server.go        Test HTTP server with 6 endpoints
       └── report.go        HTML/JSON/PDF report generation
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LOADTEST_STRAIT_URL` | `http://localhost:7676` | Strait API URL (must match `LOADTEST_PORT` in Makefile) |
| `LOADTEST_INTERNAL_SECRET` | `$INTERNAL_SECRET` | API auth secret |
| `LOADTEST_DATABASE_URL` | `$DATABASE_URL` | PostgreSQL for metrics |
| `LOADTEST_REDIS_URL` | `$REDIS_URL` | Redis for metrics |
| `LOADTEST_QUICK` | - | Set `true` for 15-min quick validation |
| `LOADTEST_TENANTS` | `500` | Tenant count for production simulation |
| `LOADTEST_DURATION` | `4h` | Duration for simulation/endurance tests |
| `LOADTEST_TARGET_RATE` | auto | Override target rate for endurance |

## Results

Results are written to `internal/loadtest/loadtest-results/<timestamp>/`:

```text
loadtest-results/2026-03-20T09-53-12/
  quick_validation.json
  throughput_ceiling.json
  concurrency_ceiling_http.json
  metrics_2026-03-20T09-53-12.jsonl
```

Generate a report:

```bash
make loadtest-report
# Output: loadtest-results/report.html, loadtest-results/report.json
```

## Running Individual Tests

If you prefer running tests directly:

```bash
# Quick validation
LOADTEST_QUICK=true go test -tags=loadtest -run TestQuickValidation -timeout 15m -v ./internal/loadtest/...

# Throughput ceiling
go test -tags=loadtest -run TestThroughputCeiling -timeout 2h -v ./internal/loadtest/...

# Concurrency ceiling
go test -tags=loadtest -run TestConcurrencyCeiling -timeout 1h -v ./internal/loadtest/...

# Specific chaos scenario
go test -tags=loadtest -run TestChaosScenarios/worker_sigkill -timeout 30m -v ./internal/loadtest/...

# Error scenarios
go test -tags=loadtest -run TestErrorScenarios -timeout 1h -v ./internal/loadtest/...

# Production validation (requires LOADTEST_STRAIT_URL pointing to deployed instance)
LOADTEST_STRAIT_URL=https://your-strait-instance go test -tags=loadtest -run TestProductionValidation -timeout 1h -v ./internal/loadtest/...
```

## Test Server Endpoints

The harness starts a local HTTP server on port 9000 with these endpoints:

| Endpoint | Behavior |
|----------|----------|
| `POST /fast-echo` | Returns payload immediately (<1ms) |
| `POST /slow-process` | 1-5s random delay |
| `POST /variable-load` | 100-2000ms with CPU busy-wait |
| `POST /flaky` | 20% failure rate, 50-250ms |
| `POST /memory-heavy` | ~100KB response |
| `POST /cost-reporter` | Simulated external service cost metadata |
| `GET /health` | Server health check |
| `GET /stats` | Request counters per endpoint |
