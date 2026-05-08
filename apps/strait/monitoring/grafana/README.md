# Strait Grafana Dashboards

This directory contains the built-in Grafana dashboards for operating Strait.

Dashboards:

- `service-overview.json` - top-level health, latency, queue, API, and dependency posture
- `api-ingress.json` - HTTP ingress latency, route rate, inflight load, and error-class volume
- `queue-health.json` - queue depth, lag, claim latency, lock contention, backpressure, and DLQ age
- `worker-execution.json` - worker pool, dispatch, retry, payload, response, and gRPC worker-plane health
- `scheduler-workflows.json` - scheduler loops, cron drift, workflow progress, durable waits, and compensation
- `triggers-webhooks.json` - event triggers, webhook delivery health, retry pressure, and breaker state
- `data-plane.json` - Postgres, replication, ClickHouse exporter, Redis pub/sub, notifications, and log drains
- `billing-cloud.json` - cloud-edition billing enforcement, Stripe ingestion, usage records, and plan gates
- `audit-events.json` - audit pipeline, SIEM forwarding, retention, export caps, and chain verification

Provisioning files under `provisioning/` expect dashboards to be mounted at
`/var/lib/grafana/dashboards/strait` and a `PROMETHEUS_URL` environment
variable for the Prometheus datasource.
