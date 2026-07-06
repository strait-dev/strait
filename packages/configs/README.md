# packages/configs

Shared configuration files for Docker-based Strait deployments (both self-hosted and local dev).

## Source Of Truth

These files are mounted by Compose. Keep operational explanations in [SELFHOST.md](../../SELFHOST.md) and [deployment docs](../../apps/docs/guides/deployment.mdx); keep this README focused on what each file does.

## Files

| File | Purpose |
|------|---------|
| `sequin.yml` | Sequin CDC configuration. Defines the Strait API token, database connection, replication slot, publication, and tracked tables for status read models plus cache repair. Mounted at `/etc/sequin/config.yml` in the Sequin container. |
| `postgres-init.sql` | Postgres bootstrap script that creates the logical replication slot (`sequin_strait_slot`), publication (`sequin_strait_pub`), and sets `REPLICA IDENTITY FULL` on CDC tables. Mounted into Postgres on first boot and rerun by the `postgres-cdc-init` service after migrations. |
| `prometheus.yml` | Optional Prometheus scrape config for the `observability` Compose profile. It reads the internal secret from the file created by the Prometheus container entrypoint. |

## Usage

Only `docker-compose.base.yml` volume-mounts these files (`postgres-init.sql`, `sequin.yml`, `prometheus.yml`); it is the shared runtime definition and owns the actual service definitions and mounts. `docker-compose.selfhost.yml` and `apps/strait/docker-compose.yml` are Compose overlays that add environment- or edition-specific overrides on top of it -- neither mounts these files itself, so it is only ever run together with the base file.

The root `Makefile` wires this up through two `-f` chains that always include the base file first:

- `DEV_COMPOSE` = `docker compose -f docker-compose.base.yml -f apps/strait/docker-compose.yml` (local development)
- `SELFHOST_COMPOSE` = `docker compose --env-file .env.selfhost -f docker-compose.base.yml -f docker-compose.selfhost.yml` (self-hosted stack)

Referenced by:
- `SELFHOST.md` (documents the auto-configured CDC setup)

## Validation

After changing Sequin, Postgres, or Prometheus config, run the relevant stack and docs checks:

```bash
make selfhost
cd apps/docs && bun run lint
```
