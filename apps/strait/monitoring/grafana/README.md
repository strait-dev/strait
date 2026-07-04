# Strait Grafana Dashboards

This directory contains the built-in Grafana dashboards for operating Strait.

## Source Of Truth

Dashboard JSON files in this directory are the checked-in source. Provisioning files under `provisioning/` define how self-host and local smoke checks load them.

Dashboards:

- `service-overview.json` - top-level health, latency, queue, API, and dependency posture
- `api-ingress.json` - HTTP ingress latency, route rate, inflight load, and error-class volume
- `queue-health.json` - queue depth, lag, claim latency, lock contention, backpressure, and DLQ age
- `worker-execution.json` - worker pool, dispatch, retry, payload, response, and gRPC worker-plane health
- `scheduler-workflows.json` - scheduler loops, cron drift, workflow progress, durable waits, and compensation
- `triggers-webhooks.json` - event triggers, webhook delivery health, retry pressure, and breaker state
- `data-plane.json` - Postgres, replication, ClickHouse exporter, Redis pub/sub, notifications, and log drains
- `cache-coherence.json` - cache hit/miss mix, fail-open rate, cachebus lag, and CAS rejects
- `billing-cloud.json` - cloud-edition billing enforcement, Stripe ingestion, usage records, and plan gates
- `audit-events.json` - audit pipeline, SIEM forwarding, retention, export caps, and chain verification

## Alert rules

Two Prometheus rule files back these dashboards:

- `../prometheus-rules.yaml` (sibling to this directory, at `monitoring/prometheus-rules.yaml`) - the main
  Prometheus rules: recording rules (`strait:*`) that pre-aggregate request rate,
  latency percentiles, queue depth, worker dispatch, workflow, scheduler, auth,
  Redis, and cache metrics for the dashboards above, plus the general alerting
  rules built on top of them.
- `audit-alerts.yaml` (in this directory) - alert rules scoped to the audit
  subsystem: deadletter growth (`AuditDLQRising`), drainer queue saturation
  (`AuditDrainerSaturated`), SIEM forwarder circuit breaker trips
  (`AuditSIEMForwardFailing`), and audit hash-chain verification failures
  (`AuditChainVerificationFailed`).

Prometheus loads both files through its `rule_files:` configuration directive;
neither is picked up automatically, so a Prometheus deployment must list both
paths explicitly.

Provisioning files under `provisioning/` expect dashboards to be mounted at
`/var/lib/grafana/dashboards/strait` and a `PROMETHEUS_URL` environment
variable for the Prometheus datasource.

Run the local provisioning smoke check with:

```bash
cd apps/strait/monitoring/grafana
./smoke.sh
```

The script starts a disposable Grafana container, loads the provisioning files,
checks the Prometheus datasource, and verifies that all ten dashboards are
available through Grafana's API with datasource and interval variables.

To compare the dashboards and alert rules against a live Strait metrics scrape:

```bash
cd apps/strait/monitoring
METRICS_URL=http://127.0.0.1:8080/metrics ./check-scrape-coverage.sh
```

The scrape coverage check reports referenced metrics that are not present in the
scrape. Set `STRICT=1` to make missing references fail the command; keep it
unset for quiet staging environments where counters may not have emitted yet.
