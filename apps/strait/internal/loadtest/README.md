# Load Testing Framework

Find where Strait breaks. Runs real workloads, measures throughput/latency/concurrency,
detects memory leaks, and simulates chaos -- all locally with Docker.

## Quick Start

From `apps/strait/`:

```bash
# 1. Start infrastructure (Postgres, Redis, Prometheus, Grafana)
make loadtest-up

# 2. Start Strait (in a separate terminal)
make loadtest-server

# 3. Run the 15-minute quick validation
make loadtest-quick

# 4. Open Grafana to see results
open http://localhost:3001
```

That's it. The quick validation ramps from 10 to ~100 jobs/sec and finds your
approximate throughput ceiling.

## All Commands

```bash
make loadtest-up           # Start Postgres + Redis + Prometheus + Grafana
make loadtest-server       # Start Strait configured for load testing
make loadtest-down         # Stop all load test infrastructure
make loadtest-quick        # 15-min quick validation (CI-friendly)
make loadtest-throughput   # Find max sustained throughput (up to 2h)
make loadtest-concurrency  # Find max concurrent connections (up to 1h)
make loadtest-endurance    # 24h stability test at 70% ceiling
make loadtest-chaos        # Run all 8 chaos engineering scenarios
make loadtest-errors       # Test all 12 error scenarios
make loadtest-all          # Run quick + throughput + concurrency
make loadtest-report       # Generate HTML/JSON report from results
make loadtest-unit         # Run unit tests for the framework itself
```

## Test Tiers

| Tier | Test | Duration | What It Finds |
|------|------|----------|---------------|
| 0 | Quick Validation | 15 min | Approximate throughput ceiling |
| 1 | Throughput Ceiling | ~1-2h | Exact max jobs/sec before breaking |
| 2 | Concurrency Ceiling | ~1h | Max concurrent connections |
| 3 | Multi-Tenant Simulation | 4-8h | Real production behavior with 500-2000 tenants |
| 3 | Breaking Point | 2-6h | Exact tenant count where system degrades |
| 4 | Endurance | 24-72h | Memory leaks, goroutine leaks, performance drift |
| 5 | Chaos Engineering | ~4h | Recovery from 8 failure scenarios |

## Grafana Dashboard

After running `make loadtest-up`, open http://localhost:3001 to see:

- Queue depth and active workers (real-time)
- Throughput and dispatch latency P50/P95/P99
- Error rates and worker pool utilization
- Database connection pool breakdown
- Webhook delivery metrics
- Go runtime: goroutines, heap memory, GC pauses

Prometheus scrapes Strait's `/metrics` endpoint every 5 seconds.

## Architecture

```
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
| `LOADTEST_STRAIT_URL` | `http://localhost:8080` | Strait API URL |
| `LOADTEST_INTERNAL_SECRET` | `$INTERNAL_SECRET` | API auth secret |
| `LOADTEST_DATABASE_URL` | `$DATABASE_URL` | PostgreSQL for metrics |
| `LOADTEST_REDIS_URL` | `$REDIS_URL` | Redis for metrics |
| `LOADTEST_QUICK` | - | Set `true` for 15-min quick validation |
| `LOADTEST_TENANTS` | `500` | Tenant count for production simulation |
| `LOADTEST_DURATION` | `4h` | Duration for simulation/endurance tests |
| `LOADTEST_TARGET_RATE` | auto | Override target rate for endurance |

## Results

Results are written to `internal/loadtest/loadtest-results/<timestamp>/`:

```
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

# Fly.io validation (requires LOADTEST_STRAIT_URL pointing to Fly deployment)
LOADTEST_STRAIT_URL=https://your-app.fly.dev go test -tags=loadtest -run TestFlyValidation -timeout 1h -v ./internal/loadtest/...
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
| `POST /cost-reporter` | Simulated LLM cost metadata |
| `GET /health` | Server health check |
| `GET /stats` | Request counters per endpoint |
