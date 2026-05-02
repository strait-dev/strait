# packages/configs

Shared configuration files for Docker-based Strait deployments (both self-hosted and local dev).

## Files

| File | Purpose |
|------|---------|
| `sequin.yml` | Sequin CDC consumer configuration. Defines the database connection, replication slot, publication, and tracked tables (`job_runs`, `workflow_runs`, `workflow_step_runs`). Mounted at `/etc/sequin/config.yml` in the Sequin container. |
| `postgres-init.sql` | Postgres bootstrap script that creates the logical replication slot (`sequin_strait_slot`), publication (`sequin_strait_pub`), and sets `REPLICA IDENTITY FULL` on CDC tables. Runs via `docker-entrypoint-initdb.d` on first boot. |

## Usage

Both files are volume-mounted by Docker Compose:

- `docker-compose.selfhost.yml` (self-hosted stack)
- `apps/strait/docker-compose.yml` (local development stack)

Referenced by:
- `SELFHOST.md` (documents the auto-configured CDC setup)
