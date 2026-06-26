# Self-Hosting Strait

This guide is for the community self-hosted edition. Use it when you need local evaluation, residency control, air-gapped operation, or direct ownership of PostgreSQL, Redis, Sequin, and the Strait process. If you want the hosted product, start with the [Cloud quickstart](apps/docs/quickstart.mdx).

Strait runs as a single Go binary alongside PostgreSQL, Redis, and Sequin services. The fastest way to stand up the full community stack on one host:

## Prerequisites

- Docker Engine with Docker Compose v2
- Git
- A POSIX-compatible shell
- Open ports `3000`, `8080`, `5432`, `6379`, and `7376` on the host, or adjust the Compose files before starting

You do not need a Strait Cloud account, Stripe account, or hosted control plane for the community stack.

## Quick Start

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

The first run creates `.env.selfhost` with random secrets. Keep that file private and back it up with the database if you intend to keep the deployment. Do not commit it.

Stop or reset:

```bash
make selfhost-down    # stop containers, keep data
make selfhost-reset   # stop and wipe data and secrets
```

Strait fails fast when PostgreSQL, Redis, or Sequin is missing or unreachable. Self-hosted PostgreSQL must run with logical replication enabled (`wal_level=logical`) so Sequin can stream changes instead of forcing Strait into polling behavior.

## Operations

Useful commands:

```bash
make selfhost                 # start API, dashboard, PostgreSQL, Redis, and Sequin
make selfhost-core            # start API stack without the dashboard
make selfhost-observability   # start with Prometheus
make selfhost-down            # stop containers and keep volumes
make selfhost-reset           # delete volumes and regenerate secrets on next start
```

Back up PostgreSQL before upgrades or host maintenance. The helper script writes compressed dumps and removes backups older than 30 days:

```bash
./packages/scripts/selfhost-backup.sh
```

For production, put TLS termination, authentication, firewalling, and backups under your own infrastructure controls. The community stack is designed to be owned by the operator; Strait Cloud support does not have access to self-hosted databases or hosts.

## Support

Use [GitHub Discussions](https://github.com/strait-dev/strait/discussions) for self-hosting questions and setup help. Use [GitHub Issues](https://github.com/strait-dev/strait/issues) for reproducible bugs. Do not post secrets, `.env.selfhost`, database dumps, API keys, or customer payloads publicly.

Report security issues privately by emailing [security@strait.dev](mailto:security@strait.dev). See [SECURITY.md](SECURITY.md).

## Full Guide

The complete self-hosting guide lives in the docs and is the canonical reference:

**https://docs.strait.dev/guides/self-hosting**

It covers dependencies, configuration, the dashboard, the first job, upgrading, and backups. For production scaling and region strategy, see https://docs.strait.dev/guides/deployment. For local development setup, see [CONTRIBUTING.md](CONTRIBUTING.md).
