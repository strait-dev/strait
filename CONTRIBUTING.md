# Contributing to Strait

## Prerequisites

- [Go](https://go.dev/dl/) 1.26+
- [Bun](https://bun.sh/) 1.3+
- [Docker](https://docs.docker.com/get-docker/) and Docker Compose v2
- [golangci-lint](https://golangci-lint.run/welcome/install/)
- [lefthook](https://github.com/evilmartians/lefthook) (git hooks)

## Setup

```bash
# Clone the repo.
git clone https://github.com/strait-dev/strait.git
cd strait

# Install frontend dependencies.
bun install

# Install git hooks.
lefthook install

# Start the dev database and Redis.
cd apps/strait && docker compose up -d

# Set up environment (copy and fill in secrets).
cp .env.example .env
# Edit .env with your values (DATABASE_URL, REDIS_URL, INTERNAL_SECRET, JWT_SIGNING_KEY)
```

## Running Locally

**Go API server:**
```bash
cd apps/strait
go run ./cmd/strait
# API at http://localhost:8080
# API docs at http://localhost:8080/reference
```

**Dashboard (TanStack Start):**
```bash
cd apps/app
bun run dev
# Dashboard at http://localhost:5173
```

`bun run dev` is local-first and does not require Doppler. It defaults to the
Docker PostgreSQL instance started from `apps/strait/docker-compose.yml` and
enables local auth/billing bypass flags so contributors can sign up and log in
without external email or Polar setup. It also bootstraps the Better Auth
schema and seeds a default local user:

- `dev@local.strait`
- `devpassword123`

**Docs site:**
```bash
cd apps/docs
bun dev
```

## Testing

```bash
cd apps/strait

# Unit tests.
go test ./... -count=1 -timeout=2m

# Unit tests with race detector.
go test ./... -race -timeout=5m

# Integration tests (requires running Postgres + Redis via docker compose).
go test -tags=integration ./internal/store/... ./internal/queue/... ./internal/e2e/...

# Frontend tests.
cd apps/app && bun test
```

## Linting

```bash
# Go linting.
cd apps/strait && golangci-lint run --timeout=5m ./...

# Frontend linting.
cd apps/app && bun run biome:lint

# Run all pre-commit checks.
lefthook run pre-commit
```

## Commit Conventions

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): summary

feat(api): add bulk trigger endpoint
fix(worker): prevent race in managed dispatch
test(store): add integration tests for job dependencies
chore: update dependencies
```

Types: `feat`, `fix`, `test`, `chore`, `refactor`, `docs`, `perf`.

Do not add AI attribution or "Co-Authored-By" lines. Do not skip git hooks (`--no-verify`).

## Pull Requests

1. Create a branch from `master`
2. Make your changes
3. Ensure all checks pass: `lefthook run pre-commit`
4. Open a PR with a clear title and description
5. CI runs Test, Lint, and Security checks automatically

## Agent Runtime Bundle

The Go backend embeds a pre-built Cloudflare Worker bundle via `go:embed` in
`apps/strait/internal/agents/cloudflare_bundle.go`. When you change agent
runtime code in `apps/agents/`, regenerate the embedded bundle:

```bash
cd apps/strait
go generate ./internal/agents/...
```

This builds the runtime worker from `apps/agents` and copies the output into
`runtime_worker_bundle.js`. The bundle is checked in because `go build` requires
embedded files to exist at compile time.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the system overview.

Key directories:
- `apps/strait/` -- Go API server, worker, scheduler
- `apps/app/` -- TanStack Start dashboard (React)
- `apps/docs/` -- Documentation site
- `apps/website/` -- Marketing website
- `packages/` -- Shared packages (UI, utils, SDK, billing)

## Self-Hosting

See [SELFHOST.md](SELFHOST.md) for running Strait on your own infrastructure.
