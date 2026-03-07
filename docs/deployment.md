# Deployment

## Prerequisites

- Go 1.26
- Docker (for testcontainers)
- golangci-lint v2

## Running Tests

```bash
# Unit tests
go test ./...

# Unit tests with race detector
go test -race ./...

# Integration tests (requires Docker for testcontainers)
go test -tags integration -race ./internal/store/... ./internal/queue/...

# E2E tests (requires Docker)
go test -tags integration -race ./internal/e2e/...

# Lint (golangci-lint v2, 18 linters)
golangci-lint run ./...

# Build
go build ./...
```

## Docker

```bash
docker build -t orchestrator .
docker run -e DATABASE_URL=... -e REDIS_URL=... -e INTERNAL_SECRET=... -e JWT_SIGNING_KEY=... -p 8080:8080 orchestrator
```

## Fly.io

```bash
fly secrets set DATABASE_URL=... REDIS_URL=... INTERNAL_SECRET=... JWT_SIGNING_KEY=...
fly deploy
```

## Scaling

Run the API and worker in separate processes for independent scaling:

```bash
# API instances (stateless, scale horizontally)
orchestrator --mode api

# Worker instances (scale based on queue depth)
orchestrator --mode worker
```

Workers use Postgres `SKIP LOCKED` for dequeuing, so multiple worker instances can run safely without double-processing. Each worker dequeues and locks its own batch of runs.
