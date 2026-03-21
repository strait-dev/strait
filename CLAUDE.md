# CLAUDE.md

Read and follow `AGENTS.md` in this repository root — it is the primary operating guide.

## Git rules (non-negotiable)

- Always use conventional commit messages: `type(scope): summary`
- Never skip lefthook hooks. Never use `--no-verify`. If a hook fails, fix the issue.
- Never add "Co-Authored-By" lines to commit messages
- Never add "Generated with Claude Code" or any AI attribution to commits or PR descriptions
- Write helpful, substantive PR descriptions about what was actually worked on

## Project quick reference

- **Language**: Go 1.26, module `strait`
- **Main app**: `apps/strait/`
- **Build**: `cd apps/strait && go build ./...`
- **Test**: `cd apps/strait && go test ./...`
- **Lint**: `cd apps/strait && golangci-lint run --timeout=5m ./...`
- **Dependencies**: PostgreSQL, Redis (see `docker-compose.yml`)
- **Env vars**: see `.env.example` and `apps/strait/internal/config/config.go`
- **Migrations**: `apps/strait/migrations/` (embedded, auto-applied on startup)
- **Doppler**: `doppler secrets --project strait --config <dev|stg|prd>`
- **Fly**: apps `strait`, `strait-sequin`, `strait-otel-collector`
- **Observability**: Grafana Cloud (metrics + logs), ClickHouse (traces + analytics), Sentry (errors), Better Stack (uptime)
- **ClickHouse**: custom analytics exporter + 12 tables (see `internal/clickhouse/schema.go`)
- **Analytics**: 32 API endpoints under `/v1/analytics/` backed by ClickHouse with Postgres fallback
- **Monitoring**: alert rules in `ops/monitoring/`, dashboards at `https://strait.grafana.net`

## Key conventions

- Raw SQL with `pgx/v5` — no ORM
- Structured concurrency with `sourcegraph/conc`
- Error wrapping with `%w` and context
- Use `apps/strait/internal/testutil` helpers in tests
- No emojis in code, comments, logs, docs, or commits
