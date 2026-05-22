# Self-hosting Strait

Strait runs as a single Go binary alongside PostgreSQL and Redis. The fastest way to stand up the full community stack on one host:

```bash
git clone https://github.com/strait-dev/strait.git
cd strait
make selfhost
```

`make selfhost` generates secrets and starts the API, dashboard, PostgreSQL, Redis, and Sequin via `docker-compose.selfhost.yml`. Then check the API and open the dashboard:

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

## Full guide

The complete self-hosting guide lives in the docs and is the canonical reference:

**https://docs.strait.dev/guides/self-hosting**

It covers the Cloudflare-hosted dashboard option, dependencies, configuration, your first job, upgrading, and backups. For production scaling and region strategy, see https://docs.strait.dev/guides/deployment. For local development setup, see [CONTRIBUTING.md](CONTRIBUTING.md).
