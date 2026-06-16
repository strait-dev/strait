# packages/scripts

Shell scripts for self-hosted Strait deployments.

## Source Of Truth

These scripts are called by root Makefile targets. User-facing setup and backup instructions live in [SELFHOST.md](../../SELFHOST.md).

## Files

| Script | Purpose |
|--------|---------|
| `selfhost-init.sh` | Generates `.env.selfhost` with random secrets, checks Docker prerequisites, and prints next-step instructions. Supports `--reset` to wipe volumes and regenerate secrets. |
| `selfhost-backup.sh` | Dumps the Strait Postgres database via `pg_dump`, compresses it with gzip, and removes backups older than 30 days. Configurable via `BACKUP_DIR` and `CONTAINER` env vars. |

## Usage

Called by Makefile targets:

```bash
make selfhost        # runs selfhost-init.sh then starts compose
make selfhost-reset  # runs selfhost-init.sh --reset
```

`selfhost-backup.sh` is intended for cron scheduling:

```text
0 3 * * * cd /path/to/strait && ./packages/scripts/selfhost-backup.sh
```

Referenced by:
- `Makefile` (`selfhost`, `selfhost-reset` targets)
- `SELFHOST.md` (self-host guide)
- `docker-compose.selfhost.yml` (comment header)

## Validation

```bash
shellcheck packages/scripts/*.sh
cd apps/docs && bun run lint
```

If `shellcheck` is not installed locally, run the self-host flow before committing script changes:

```bash
make selfhost
make selfhost-down
```
