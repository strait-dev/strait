# Strait

Durable workflow orchestrator with secure sandboxed code execution.

## Architecture

Strait is a monorepo containing two services:

| App | Language | Purpose |
|-----|----------|---------|
| [**Strait**](apps/strait/) | Go | Workflow orchestrator — manages jobs, workflows, queues, retries, and execution lifecycle |
| [**Forge**](apps/forge/) | Elixir | Sandbox executor — runs user code in isolated environments via gRPC |

Communication between Strait and Forge happens over **gRPC**, defined in [`packages/proto/`](packages/proto/).

```
strait/
├── apps/
│   ├── strait/              # Go orchestrator
│   └── forge/               # Elixir sandbox service
├── packages/
│   └── proto/               # Shared protobuf definitions
├── turbo.json               # Turborepo pipeline config
├── docker-compose.yml       # Full local stack
└── package.json             # Bun workspace root
```

## Prerequisites

- [Go](https://go.dev/) 1.26+
- [Elixir](https://elixir-lang.org/) 1.17+ / OTP 27+
- [Bun](https://bun.sh/) 1.3+
- [Docker](https://www.docker.com/) (for local infra)
- [protoc](https://grpc.io/docs/protoc-installation/) (for proto codegen)

## Getting Started

### Install dependencies

```bash
bun install
```

### Start local infrastructure

```bash
docker compose up -d postgres redis sequin
```

### Run everything

```bash
# Build all apps
bun run build

# Test all apps
bun run test

# Lint all apps
bun run lint
```

### Run individual apps

```bash
# Strait (Go orchestrator)
bun run --filter=@strait/orchestrator dev

# Forge (Elixir sandbox)
bun run --filter=@strait/forge dev
```

### Run with Docker Compose

```bash
# Start the full stack (Postgres + Redis + Sequin + Strait + Forge)
docker compose up
```

## Protobuf

Shared gRPC definitions live in `packages/proto/`. After modifying `.proto` files:

```bash
bun run proto:gen
```

This generates:
- Go stubs in `apps/strait/internal/sandbox/`
- Elixir stubs in `apps/forge/lib/proto/`

## Development

### Turborepo

This repo uses [Turborepo](https://turbo.build/) with [Bun](https://bun.sh/) workspaces. Each app has a `package.json` that wraps native build commands so Turborepo can orchestrate them.

```bash
# Run a task for a specific app
bun run --filter=@strait/orchestrator test
bun run --filter=@strait/forge test

# Run all tasks
bun run test
```

### Testing

- **Strait**: `cd apps/strait && go test -race ./...`
- **Forge**: `cd apps/forge && mix test`
- **Integration tests (Strait)**: `cd apps/strait && go test -tags=integration ./internal/store ./internal/queue ./internal/pubsub ./internal/e2e`

### CI

CI runs on GitHub Actions with separate jobs for:
- **Lint**: Go (golangci-lint), Elixir (credo), Proto (buf)
- **Test**: Go unit + race + coverage, Go integration, Elixir tests

## License

See [LICENSE](apps/strait/LICENSE).
