# Contributing to Strait

Thanks for sending a patch. This guide gets a development environment running and explains how PRs land.

If you only want to run Strait, not develop on it, follow [SELFHOST.md](SELFHOST.md) instead.

## Prerequisites

- [Go](https://go.dev/dl/) 1.26+
- [Bun](https://bun.sh/) 1.3+
- [Docker](https://docs.docker.com/get-docker/) and Docker Compose v2
- [golangci-lint](https://golangci-lint.run/welcome/install/)
- [lefthook](https://github.com/evilmartians/lefthook) for git hooks

## Setup

```bash
# Clone the repo.
git clone https://github.com/strait-dev/strait.git
cd strait

# Install frontend dependencies.
bun install

# Install git hooks.
lefthook install

# Set up environment (copy and fill in secrets).
cp .env.example .env
# Edit .env. The defaults already point at the dev compose ports:
#   DATABASE_URL=postgres://strait:strait@localhost:15432/strait?sslmode=disable
#   REDIS_URL=redis://localhost:16379

# Start Postgres, Redis, and Sequin (CDC).
# Postgres maps to host port 15432, Redis to 16379 (avoids clashing
# with Postgres/Redis already running on your machine).
cd apps/strait && docker compose up -d
```

## Running Locally

**Go API server:**
```bash
cd apps/strait
go run ./cmd/strait --mode all
# API at http://localhost:8080
# API docs at http://localhost:8080/reference
```

**Dashboard (TanStack Start):**
```bash
bun run --cwd apps/app dev
# Dashboard at http://localhost:5173
```

**Docs site:**
```bash
bun run --cwd apps/docs dev
```

## Testing

```bash
cd apps/strait

# Unit tests.
go test ./...

# Unit tests with race detector.
go test -race ./...

# Integration tests (requires running Postgres + Redis via docker compose).
go test -tags=integration ./...

# Frontend tests (Vitest; `bun test` runs Bun's own runner and skips the suite).
cd apps/app && bun run test
```

## Building

Both editions must compile before pushing (edition is a compile-time build tag):

```bash
cd apps/strait
go build ./...            # community
go build -tags cloud ./...  # cloud
```

## Linting

```bash
# Go linting.
cd apps/strait && golangci-lint run --timeout=10m ./...

# Frontend linting.
cd apps/app && bun run biome:lint

# Run all pre-commit checks.
lefthook run pre-commit
```

## Documentation checks

Run these when changing `apps/docs`, `README.md`, or other public Markdown:

```bash
bun run --cwd apps/docs lint
git diff --check
```

The docs linter checks MDX structure, internal links, anchors, example hosts, billing catalog drift, documented routes, documented environment variables, run states, webhook events, and first-party Markdown links.

## Markdown scope

Keep customer documentation in `apps/docs`, `README.md`, and `SELFHOST.md`. These files should explain what customers can do with Strait and how to do it.

Package READMEs are for contributors and should stay short: purpose, entry points, commands, and ownership context. Do not add launch plans, billing cutovers, incident notes, private checklists, or internal runbooks to the repo. Keep those outside the public repository.

## Support expectations

Use GitHub Discussions for questions, troubleshooting, and design discussion. Use GitHub Issues for reproducible bugs and public feature requests.

Do not post secrets, customer data, private logs, database dumps, `.env` files, or vulnerability details in public issues or discussions. Send vulnerability reports to [security@strait.dev](mailto:security@strait.dev); see [SECURITY.md](SECURITY.md).

## Commit Conventions

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): summary

feat(api): add bulk trigger endpoint
fix(worker): prevent race in gRPC worker dispatch
test(store): add integration tests for job dependencies
chore: update dependencies
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`, `revert`, `style`.

Do not add AI attribution or "Co-Authored-By" lines. Do not skip git hooks (`--no-verify`).

## Pull requests

Branch from `master`, make your changes, run `lefthook run pre-commit`, and open a PR with a clear description. CI runs the test, lint, and security suites for you.

If the change is large or touches multiple subsystems, open an issue first so we can agree on the approach before you write the code.

## Architecture

See the [architecture docs](apps/docs/architecture.mdx) and [AGENTS.md](AGENTS.md) for a system overview.

Key directories:
- `apps/strait/`: Go API server, worker, scheduler.
- `apps/app/`: TanStack Start dashboard (React).
- `apps/docs/`: Documentation site.
- `packages/`: Shared TS workspaces (`billing`, `config`, `transactional`) plus repo config and ops helpers (`configs`, `monitoring`, `scripts`). The `@strait/ui` component library is an external npm package.

The marketing website lives in its own repo: <https://github.com/strait-dev/website>

## Self-Hosting

See [SELFHOST.md](SELFHOST.md) for running Strait on your own infrastructure.
