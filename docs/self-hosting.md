# Self-Hosting Strait

Run the full Strait platform on your own infrastructure with a single command.

## Quick Start

```bash
git clone https://github.com/leonardomso/strait.git
cd strait
docker compose -f docker-compose.self-host.yml up -d
```

Strait is now running at `http://localhost:8080`.

## Service URLs

| Service    | URL                      | Notes                        |
|------------|--------------------------|------------------------------|
| Strait API | http://localhost:8080     | REST API and health endpoint |
| Grafana    | http://localhost:3001     | Dashboards and monitoring    |
| Prometheus | http://localhost:9090     | Metrics collection           |

## Default Credentials

| Service  | Username | Password |
|----------|----------|----------|
| Grafana  | admin    | admin    |
| Postgres | strait   | strait   |

Change these before running in production.

## Environment Variables

Override defaults by setting environment variables before starting the stack, or
by creating a `.env` file next to the compose file.

| Variable                | Default                                              | Description                          |
|-------------------------|------------------------------------------------------|--------------------------------------|
| `INTERNAL_SECRET`       | `change-this-internal-secret-at-least-32-chars`      | Secret for internal service calls    |
| `JWT_SIGNING_KEY`       | `change-this-jwt-signing-key-at-least-32-chars`      | JWT signing key (min 32 chars)       |
| `STRAIT_EDITION`        | `community`                                          | Edition: `community` or `cloud`      |
| `MODE`                  | `all`                                                | Run mode: `all`, `api`, or `worker`  |
| `LOG_LEVEL`             | `info`                                               | Log level: `debug`, `info`, `warn`   |
| `WORKER_CONCURRENCY`    | `25`                                                 | Number of concurrent workers         |
| `DB_MAX_CONNS`          | `50`                                                 | Max database connections             |
| `WEBHOOK_REQUIRE_TLS`   | `false`                                              | Require TLS for webhook endpoints    |
| `ALLOW_PRIVATE_ENDPOINTS` | `true`                                             | Allow webhooks to private IPs        |

See `.env.example` for the full list of configuration options.

## Edition Comparison

| Feature                        | Community | Cloud |
|--------------------------------|-----------|-------|
| HTTP job dispatch              | Yes       | Yes   |
| Cron scheduling                | Yes       | Yes   |
| Workflow orchestration         | Yes       | Yes   |
| Webhook delivery               | Yes       | Yes   |
| Event triggers                 | Yes       | Yes   |
| Basic analytics (7 endpoints)  | Yes       | Yes   |
| Prometheus metrics             | Yes       | Yes   |
| Health checks                  | Yes       | Yes   |
| RBAC and scoped API keys       | Yes       | Yes   |
| Advanced analytics (25 endpoints) | --     | Yes   |
| Managed container execution    | --        | Yes   |
| Multi-region execution         | --        | Yes   |
| Cost forecasting               | --        | Yes   |
| Warm pool management           | --        | Yes   |

Cloud-only endpoints return `402 Payment Required` on the community edition.

## Production Hardening

Before running in production, address the following:

- **Change default secrets**: Set unique values for `INTERNAL_SECRET` and `JWT_SIGNING_KEY`.
- **Change database credentials**: Override `POSTGRES_USER` and `POSTGRES_PASSWORD`.
- **Enable TLS**: Put a reverse proxy (nginx, Caddy, Traefik) in front of the Strait API with TLS termination.
- **Set `WEBHOOK_REQUIRE_TLS=true`**: Enforce TLS for outbound webhook delivery.
- **Set `ALLOW_PRIVATE_ENDPOINTS=false`**: Prevent webhooks from targeting internal IPs.
- **Tune connection pools**: Adjust `DB_MAX_CONNS`, `DB_MIN_CONNS`, and `WORKER_CONCURRENCY` for your workload.
- **External Postgres**: Point `DATABASE_URL` at a managed Postgres instance with backups enabled.
- **External Redis**: Point `REDIS_URL` at a managed Redis instance for persistence.
- **Resource limits**: Add `deploy.resources.limits` to the compose file for CPU and memory.

## Upgrading

Pull the latest images and recreate containers:

```bash
docker compose -f docker-compose.self-host.yml pull
docker compose -f docker-compose.self-host.yml up -d
```

Migrations run automatically on startup. The `MIGRATION_MODE` variable controls
this behavior (`auto`, `manual`, or `validate`).

## Backup and Restore

### Backup

```bash
docker compose -f docker-compose.self-host.yml exec postgres \
  pg_dump -U strait strait > strait-backup-$(date +%Y%m%d).sql
```

### Restore

```bash
docker compose -f docker-compose.self-host.yml exec -T postgres \
  psql -U strait strait < strait-backup-20260321.sql
```

## Verifying Your Setup

Check the health endpoint to confirm the edition:

```bash
curl http://localhost:8080/health
# {"status":"ok","edition":"community"}
```

Attempt a cloud-only endpoint to verify gating:

```bash
curl http://localhost:8080/v1/analytics/runs/timeline
# 402 {"error":"this feature requires Strait Cloud","edition":"community","upgrade":"https://strait.dev/pricing"}
```
