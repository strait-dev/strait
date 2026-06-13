# Self-Hosting Strait

This guide is for the community self-hosted edition. Use it when you need local evaluation, residency control, air-gapped operation, or direct ownership of PostgreSQL, Redis, Sequin, and the Strait process. If you want the hosted product, start with the [Cloud quickstart](apps/docs/quickstart.mdx).

Strait runs as a single Go binary alongside PostgreSQL, Redis, and Sequin services. The fastest way to stand up the full community stack on one host:

```bash
git clone https://github.com/strait-dev/strait.git
cd strait
make selfhost
```

`make selfhost` generates secrets and starts the API, dashboard, PostgreSQL, Redis, and Sequin via `docker-compose.base.yml` plus `docker-compose.selfhost.yml`. Then check the API and open the dashboard:

```bash
curl http://localhost:8080/health
# {"edition":"community","status":"ok"}

open http://localhost:3000
```

Stop or reset:

```bash
make selfhost-down    # stop containers, keep data
make selfhost-reset   # stop and wipe data and secrets
```

Strait fails fast when PostgreSQL, Redis, or Sequin is missing or unreachable. Self-hosted PostgreSQL must run with logical replication enabled (`wal_level=logical`) so Sequin can stream changes instead of forcing Strait into polling behavior.

## Full Guide

The complete self-hosting guide lives in the docs and is the canonical reference:

**https://docs.strait.dev/guides/self-hosting**

It covers dependencies, configuration, the dashboard, the first job, upgrading, and backups. For production scaling and region strategy, see https://docs.strait.dev/guides/deployment. For local development setup, see [CONTRIBUTING.md](CONTRIBUTING.md).
