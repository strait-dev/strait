# packages/deploy

Self-host deployment infrastructure for the Strait stack. Prometheus and Grafana configurations used by `docker-compose.selfhost.yml` so a fresh self-host install ships with metrics and a dashboard.

## Source Of Truth

Runtime mounting lives in `docker-compose.selfhost.yml`. Customer setup instructions live in [SELFHOST.md](../../SELFHOST.md). This package only owns the deployable files.

## Structure

```
self-host/
  prometheus.yml          # Scrape config targeting strait:8080/metrics (15s interval)
  grafana/
    dashboards/
      strait-overview.json  # Pre-built Grafana dashboard
    provisioning/
      dashboards/           # Grafana dashboard provisioning
      datasources/          # Grafana datasource provisioning (Prometheus)
```

## Usage

These files are volume-mounted by the self-host Docker Compose stack. Prometheus scrapes the Strait app metrics endpoint; Grafana auto-provisions the datasource and dashboard on first boot.

Referenced by:
- `docker-compose.selfhost.yml` (Prometheus and Grafana service volumes)
- `apps/docs/guides/deployment.mdx` (production deploy docs)

## Validation

```bash
make selfhost
cd apps/docs && bun run lint
```
