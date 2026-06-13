# packages/monitoring

Prometheus alerting rules and Grafana dashboards for Strait production monitoring.

## Source Of Truth

This package owns shared production rules and dashboards. Built-in Go service dashboards live under `apps/strait/monitoring/grafana`.

## Files

| File | Purpose |
|------|---------|
| `alerts-strait-core.yaml` | Alert rules for dispatch errors, queue depth, latency, consumer lag, WAL growth, replication slot health, and more (189 lines, ~15 rules) |
| `alerts-authz-rbac.yaml` | Alert rules for authorization failures (403 rate), permission cache hit ratio, and audit event insert errors |
| `grafana-authz-rbac-dashboard.json` | Grafana dashboard for AuthZ/RBAC metrics (HTTP 401/403 rates, cache performance) |

## Alert groups

- **strait-core** -- dispatch health, queue health, consumer lag, WAL/replication, connection pool, error rates
- **strait-authz-rbac** -- forbidden rate spikes, permission cache churn, audit write failures

## Usage

These files are loaded by Prometheus (`rule_files` directive) and Grafana (dashboard provisioning) in production and self-hosted deployments. They are not referenced directly by application code.

## Validation

When alert rules or dashboards change, validate both the files and docs that mention monitoring:

```bash
cd apps/docs && bun run lint
```

If a live Strait instance is available:

```bash
cd apps/strait/monitoring
METRICS_URL=http://127.0.0.1:8080/metrics ./check-scrape-coverage.sh
```
