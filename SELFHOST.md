# Self-hosting Strait

Two ways to self-host. Pick the one that matches where you want the dashboard to run.

| | Cloudflare dashboard | Full Docker stack |
|---|---|---|
| Setup | One click | `make selfhost` |
| Dashboard runs on | Your own Cloudflare account | Your own hardware |
| API runs on | Your own infrastructure (anywhere reachable) | Docker Compose on the same host |
| Postgres | Neon, Supabase, or any hosted or self-hosted PostgreSQL | Bundled `postgres:18-alpine` |
| Best for | Fast setup, teams already on Cloudflare | Air-gapped, on-prem, or Docker-first teams |

Both options run the community edition. The hosted Strait Cloud service is not required.

---

## Option 1: Deploy the dashboard to Cloudflare

[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=https://github.com/strait-dev/strait)

Click the button to fork the repo and start a Cloudflare Workers Builds import. Because Cloudflare does not yet support Bun workspaces in one-click deploys, set **Root directory** to `apps/app` and provide a custom **Build command** during import.

See [apps/app/README.md](apps/app/README.md#deploy-to-cloudflare) for the exact build command and the secrets to set after the first deploy.

**The dashboard needs the Strait API server.** Run the API locally with Option 2 below, on a VPS, or on any host reachable by Cloudflare. Then point `STRAIT_API_URL` at it in the Cloudflare Worker variables.

---

## Option 2: Run the full stack locally with Docker Compose

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/) v2+
- 2 GB RAM minimum (4 GB recommended)
- Ports 3000 (Dashboard), 8080 (API), 5432 (Postgres), and 6379 (Redis) available

### Quick start

```bash
# 1. Clone the repository.
git clone https://github.com/strait-dev/strait.git
cd strait

# 2. Generate secrets and start all services.
make selfhost

# 3. Check the API.
curl http://localhost:8080/health
# {"edition":"community","status":"ok"}

# 4. Open the dashboard.
open http://localhost:3000
```

If you prefer the underlying commands, `make selfhost` runs `./packages/scripts/selfhost-init.sh` and then starts `docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml up -d`.

To stop or reset the stack:

```bash
make selfhost-down
make selfhost-reset
```

---

## Creating your first job

These steps work the same regardless of which option you picked.

Open `http://localhost:3000`, sign up, and click **Create project** in the getting-started wizard. The wizard walks you through installing an SDK and triggering your first run.

### API-only setup

If you'd rather skip the dashboard, this script creates a project, an API key, and a job, then triggers it:

```bash
SECRET="<your INTERNAL_SECRET from .env.selfhost>"

# Create a project.
curl -X POST http://localhost:8080/v1/projects \
  -H "Content-Type: application/json" \
  -H "X-Internal-Secret: $SECRET" \
  -d '{"id": "my-project", "org_id": "my-org", "name": "My Project"}'

# Create an API key.
API_KEY=$(curl -s -X POST http://localhost:8080/v1/api-keys \
  -H "Content-Type: application/json" \
  -H "X-Internal-Secret: $SECRET" \
  -d '{"project_id": "my-project", "name": "dev-key"}' | jq -r '.key')

# Create and trigger a job.
JOB=$(curl -s -X POST http://localhost:8080/v1/jobs \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"project_id":"my-project","name":"My Job","slug":"my-job","endpoint_url":"https://httpbin.org/post","max_attempts":3,"timeout_secs":30}')

curl -X POST "http://localhost:8080/v1/jobs/$(echo $JOB | jq -r .id)/trigger" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"payload": {"hello": "world"}}'
```

## What gets deployed

| Service | Image | Port | Purpose |
|---|---|---|---|
| `strait` | `ghcr.io/strait-dev/strait:0.1.6` | 8080 | API server and worker | <!-- x-release-please-version -->
| `strait-app` | `ghcr.io/strait-dev/strait-app:0.1.6` | 3000 | Dashboard UI | <!-- x-release-please-version -->
| `postgres` | `postgres:18-alpine` | 5432 | Primary database |
| `redis` | `redis:8-alpine` | 6379 | Pub/sub and caching |

`docker-compose.selfhost.yml` pins to the exact release tags listed above, so a `docker compose up` always reproduces a known-good combination. Override with `STRAIT_IMAGE` or `STRAIT_APP_IMAGE` to track `:latest`, `:0.1`, `:0`, or any specific version.

## Community edition

Self-hosting runs the community edition. It includes job creation, scheduling, HTTP and gRPC worker dispatch, retry strategies, workflows, webhook delivery, live run updates, API keys, RBAC, run management, and `dead_letter` replay.

The hosted service at [strait.dev](https://strait.dev) adds managed infrastructure, usage metering, and hosted reporting. Your job code still runs on your own infrastructure on either edition.

## Dependencies

| Dependency | Required for |
|---|---|
| PostgreSQL 18+ | Source of truth and queue (`SELECT ... FOR UPDATE SKIP LOCKED`) |
| Redis 8+ | Pub/sub, SSE streaming, gRPC worker plane signalling |

PostgreSQL is the only hard requirement: Strait will start without Redis, but SSE, live run updates, and the gRPC worker plane will not work. Run with Redis unless you have a specific reason not to.

## Environment overrides

Override defaults by editing `.env.selfhost` or by adding entries to the `environment:` section of `docker-compose.selfhost.yml`. The most common ones:

| Variable | Default | Description |
|---|---|---|
| `WORKER_CONCURRENCY` | `25` | Max concurrent job executions |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `RATE_LIMIT_REQUESTS` | `100` | Global rate limit per minute per IP |
| `DEFAULT_JOB_TIMEOUT_SECS` | `300` | Default job timeout |
| `DEFAULT_JOB_MAX_ATTEMPTS` | `3` | Default retry attempts |

## Upgrading

```bash
docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml pull
docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml up -d
```

Migrations run automatically on startup.

## Backup

Use the included backup script:

```bash
# Create a backup.
./packages/scripts/selfhost-backup.sh
# Backup saved to ./backups/strait_20260325_120000.sql.gz

# Schedule daily backups (add to crontab).
# 0 3 * * * cd /path/to/strait && ./packages/scripts/selfhost-backup.sh
```

Backups older than 30 days are automatically cleaned up.

Restore from backup:

```bash
gunzip -c backups/strait_20260325_120000.sql.gz | docker exec -i strait-postgres psql -U strait strait
```

## API reference

Interactive API documentation is available via Scalar:

- **Self-hosted**: http://localhost:8080/reference
- **Cloud**: https://api.strait.dev/reference

The OpenAPI 3.0 spec is served at `/reference/openapi.json`.

## Monitoring

Strait exposes Prometheus-compatible metrics at the `/metrics` endpoint. Point your existing Prometheus, Datadog, or New Relic scraper at `http://localhost:8080/metrics`.

For hosted reporting, usage history, log search, and alerting, use [Strait Cloud](https://strait.dev).

## Using the SDKs

Trigger jobs programmatically from your application:

**TypeScript** ([github.com/strait-dev/strait-ts](https://github.com/strait-dev/strait-ts)):
```bash
npm install @strait/ts
```
```typescript
import { Strait } from "@strait/ts";

const strait = new Strait({
  baseUrl: "http://localhost:8080",
  apiKey: "strait_...",
});

const run = await strait.jobs.trigger("my-job", {
  payload: { userId: "123", action: "process" },
});
```

**Python** ([github.com/strait-dev/strait-python](https://github.com/strait-dev/strait-python)):
```bash
pip install strait
```
```python
from strait import Strait

client = Strait(base_url="http://localhost:8080", api_key="strait_...")
run = client.jobs.trigger("my-job", payload={"userId": "123"})
```

**Go** ([github.com/strait-dev/strait-go](https://github.com/strait-dev/strait-go)):
```bash
go get github.com/strait-dev/strait-go
```
```go
client := strait.New("http://localhost:8080", "strait_...")
run, _ := client.Jobs.Trigger(ctx, "my-job", map[string]any{"userId": "123"})
```

Or use the REST API directly:
```bash
curl -X POST http://localhost:8080/v1/jobs/my-job/trigger \
  -H "Authorization: Bearer strait_..." \
  -H "Content-Type: application/json" \
  -d '{"payload": {"userId": "123"}}'
```

## Resetting

Start fresh by wiping all data and secrets:

```bash
./packages/scripts/selfhost-init.sh --reset
./packages/scripts/selfhost-init.sh
docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml up -d
```

## Troubleshooting

### Strait API will not start

1. Read the logs:
   ```bash
   docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml logs strait
   ```
2. Confirm Postgres is healthy with `docker ps`. The API retries the database for a few seconds on startup; if Postgres is still booting, give it a moment.
3. If the secrets in `.env.selfhost` look corrupted, regenerate them:
   ```bash
   make selfhost-reset
   make selfhost
   ```

### Dashboard cannot reach the API

Both services must share the same `INTERNAL_SECRET` from `.env.selfhost`. Fix the value, then restart both:

```bash
docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml restart
```

### Migrations failed

Migrations run automatically on startup. Find the failing one in the logs:

```bash
docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml logs strait | grep migration
```

For development setup, see [CONTRIBUTING.md](CONTRIBUTING.md).
