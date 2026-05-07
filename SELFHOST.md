# Self-hosting Strait

There are two ways to self-host. Pick the one that matches how you want to run the dashboard.

| | Option 1 — Cloudflare dashboard | Option 2 — Full Docker stack |
|---|---|---|
| Setup | One click | `make selfhost` |
| Dashboard runs on | Your own Cloudflare account | Your own hardware |
| API runs on | Your own infrastructure (anywhere reachable) | Docker Compose on the same host |
| Postgres | Neon / Supabase / any hosted or self-hosted PostgreSQL | Bundled `postgres:18-alpine` |
| Best for | Zero-ops setups, teams already on Cloudflare | Air-gapped, on-prem, purist Docker users |

Both run the community edition. Every open-source feature is available on either path.

---

## Option 1 — Deploy the dashboard to Cloudflare

[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=https://github.com/strait-dev/strait)

Click the button to fork the repo and start a Cloudflare Workers Builds import. Because Cloudflare doesn't yet support Bun workspaces in one-click deploys, you'll need to set **Root directory** to `apps/app` and provide a custom **Build command** during import. The rest of the flow is guided -- Hyperdrive binding, non-secret variables, and secret prompts are predeclared in `apps/app/wrangler.jsonc`.

See [apps/app/README.md](apps/app/README.md#deploy-to-cloudflare) for the detailed walkthrough, including the exact build command string and the list of secrets you must set in the Cloudflare dashboard after the first deploy.

**The dashboard needs the Strait API server to function.** Run it locally with Option 2 below, on a VPS, or on any service host reachable by Cloudflare -- then point `STRAIT_API_URL` at it in the Cloudflare Worker's Variables. The easiest way to expose a local API is via `cloudflared tunnel` or a public hostname.

---

## Option 2 — Run the full stack locally with Docker Compose

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/) v2+
- 2 GB RAM minimum (4 GB recommended)
- Ports 3000 (Dashboard), 8080 (API), 5432 (Postgres), 6379 (Redis), 7376 (Sequin) available

### Quick Start

```bash
# 1. Clone the repository.
git clone https://github.com/strait-dev/strait.git
cd strait

# 2. Generate secrets and start all services.
make selfhost

# 3. Verify.
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

Open `http://localhost:3000`, sign up — a workspace is created for you on first login — and click **Create project** in the getting-started wizard. The wizard walks you through installing an SDK and triggering your first run.

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

## What Gets Deployed

| Service | Image | Port | Purpose |
|---|---|---|---|
| `strait` | `ghcr.io/strait-dev/strait:latest` | 8080 | API server + worker |
| `strait-app` | `ghcr.io/strait-dev/strait-app:latest` | 3000 | Dashboard UI |
| `postgres` | `postgres:18-alpine` | 5432 | Primary database |
| `redis` | `redis:8-alpine` | 6379 | Pub/sub, caching |
| `sequin` | `sequin/sequin:latest` | 7376 | CDC (Change Data Capture) |

## Community edition

Self-hosting runs the community edition. It includes job creation and scheduling, HTTP and gRPC worker dispatch, cron with overlap policies, retry strategies (exponential, linear, fixed, custom), workflow DAGs with dependencies, webhook subscriptions and delivery, SSE streaming, API keys and RBAC, full run-lifecycle management (cancel, replay, pause, resume), and the dead-letter queue for runs that exhaust their retries.

The hosted orchestrator at [strait.dev](https://strait.dev) adds multi-region orchestration, advanced analytics, and Stripe-backed metering. Your job code still runs on your own infrastructure on either edition.

## Real-time updates (CDC)

Strait uses [Sequin](https://sequinstream.com) for change data capture. Sequin watches four PostgreSQL tables — `job_runs`, `workflow_runs`, `workflow_step_runs`, and `event_triggers` — and pushes changes back to Strait through an HTTP pull consumer. That's what powers the SSE stream at `GET /v1/runs/{runID}/stream` and the live dashboard.

The consumer is provisioned by `packages/configs/sequin.yml`, mounted into the Sequin container at boot. Nothing to configure by hand. Verify with:

```bash
curl http://localhost:7376/health
```

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

## API Reference

Interactive API documentation is available via Scalar:

- **Self-hosted**: http://localhost:8080/reference
- **Cloud**: https://api.strait.dev/reference

The OpenAPI 3.0 spec is served at `/reference/openapi.json`.

## Audit tamper-evidence hardening

Strait's audit log is HMAC-signed — each entry carries a keyed hash that proves it has not been altered, so tampering is forensically detectable. For defense-in-depth, restrict the application's database role to insert-only access on the `audit_events` table. That prevents a compromised process from modifying or deleting audit history.

See migration `000187_audit_events_dml_restrictions` for the exact setup, or check the [Mintlify docs](https://docs.strait.dev) for a step-by-step walkthrough. The `/health/ready` endpoint reports `audit_dml_guard: ok` once enforced.

## Monitoring

Strait exposes Prometheus-compatible metrics at the `/metrics` endpoint. Point your existing Prometheus, Datadog, or New Relic scraper at `http://localhost:8080/metrics`.

For advanced analytics dashboards, cost tracking, log search, and alerting -- use [Strait Cloud](https://strait.dev).

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

**Strait API won't start.** Check the logs first: `docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml logs strait`. Usual culprits are PostgreSQL not yet ready (give it a moment, then retry) or bad secrets in `.env.selfhost`. Run `docker ps` to confirm Postgres is healthy. If the secrets look corrupted, regenerate them with `make selfhost-reset` followed by `make selfhost`.

**Dashboard can't reach the API.** Both services must share the same `INTERNAL_SECRET` from `.env.selfhost`. After fixing it, restart both: `docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml restart`.

**Sequin won't start.** Logs: `docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml logs sequin`. Sequin needs PostgreSQL with logical replication enabled — the Alpine Postgres image enables it by default, so the usual fix is just waiting for Postgres to finish coming up.

**Migrations failed.** They run automatically on startup. Look at `docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml logs strait | grep migration` to see which one and why.

For development setup, see [CONTRIBUTING.md](CONTRIBUTING.md).
