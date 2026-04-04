.PHONY: help setup dev dev-stripe app app-migrate selfhost selfhost-down selfhost-reset build test test-race test-int test-load lint vet bench migrate-up migrate-down migrate-status migrate-create clean check

help:
	@echo "Strait Development Commands"
	@echo ""
	@echo "Setup & Dependencies:"
	@echo "  make setup              Download dependencies, tidy modules, install git hooks"
	@echo ""
	@echo "Development:"
	@echo "  make dev                Start Docker dependencies and run API server (community edition)"
	@echo "  make dev-stripe         Start Docker, Stripe webhook forwarding, and run API server with Doppler"
	@echo "  make app                Start the frontend dashboard (Vite dev server)"
	@echo "  make app-migrate        Run Better Auth database migrations"
	@echo "  make selfhost           Generate self-host secrets and start the self-host stack"
	@echo "  make selfhost-down      Stop the self-host stack"
	@echo "  make selfhost-reset     Tear down the self-host stack and regenerate secrets on next start"
	@echo "  make build              Build all packages"
	@echo ""
	@echo "Testing:"
	@echo "  make test               Run all tests"
	@echo "  make test-race          Run tests with race detector"
	@echo "  make test-int           Run integration tests"
	@echo "  make test-load          Run load tests (30m timeout)"
	@echo ""
	@echo "Code Quality:"
	@echo "  make lint               Run golangci-lint"
	@echo "  make vet                Run go vet"
	@echo "  make check              Run build, vet, test, and lint"
	@echo ""
	@echo "Performance:"
	@echo "  make bench              Run benchmarks with memory stats"
	@echo ""
	@echo "Database:"
	@echo "  make migrate-up         Apply pending migrations"
	@echo "  make migrate-down       Rollback last migration"
	@echo "  make migrate-status     Show migration status"
	@echo "  make migrate-create     Create new migration (NAME=migration_name)"
	@echo ""
	@echo "Cleanup:"
	@echo "  make clean              Clean test cache and stop Docker containers"

setup:
	cd apps/strait && go mod download
	cd apps/strait && go mod tidy
	lefthook install

dev:
	docker compose -f apps/strait/docker-compose.yml up -d
	cd apps/strait && go run ./cmd/strait --mode all

dev-stripe:
	docker compose -f apps/strait/docker-compose.yml up -d
	@echo "Running Better Auth migrations..."
	@doppler run --project strait --config dev -- sh -c 'cd apps/app && bun run db:migrate'
	@echo ""
	@echo "Syncing Doppler secrets to apps/app/.dev.vars..."
	@doppler secrets download --project strait --config dev --no-file --format env > apps/app/.dev.vars
	@echo ""
	@echo "Starting Stripe webhook forwarding in background..."
	@stripe listen --forward-to localhost:8080/webhooks/stripe > /tmp/stripe-listen.log 2>&1 & echo $$! > /tmp/stripe-listen.pid
	@sleep 2
	@echo "Stripe webhooks forwarding to localhost:8080/webhooks/stripe (PID: $$(cat /tmp/stripe-listen.pid))"
	@echo "Logs at /tmp/stripe-listen.log"
	@echo ""
	doppler run --project strait --config dev -- sh -c 'cd apps/strait && go run ./cmd/strait --mode all'
	@-kill $$(cat /tmp/stripe-listen.pid) 2>/dev/null; rm -f /tmp/stripe-listen.pid

app:
	@echo "Syncing Doppler secrets to apps/app/.dev.vars..."
	@doppler secrets download --project strait --config dev --no-file --format env > apps/app/.dev.vars
	cd apps/app && bun dev

app-migrate:
	@echo "Running Better Auth migrations..."
	@doppler run --project strait --config dev -- sh -c 'cd apps/app && bun run db:migrate'

selfhost:
	@./packages/scripts/selfhost-init.sh
	docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml up -d

selfhost-down:
	docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml down

selfhost-reset:
	@./packages/scripts/selfhost-init.sh --reset

build:
	cd apps/strait && go build ./...

test:
	cd apps/strait && go test ./...

test-race:
	cd apps/strait && go test -race ./...

test-int:
	cd apps/strait && go test -tags integration ./...

test-load:
	cd apps/strait && go test -tags loadtest,integration -v -timeout=30m ./test/loadtest/...

lint:
	cd apps/strait && golangci-lint run --timeout=5m ./...

vet:
	cd apps/strait && go vet ./...

bench:
	cd apps/strait && go test -bench . -benchmem -run=^$$ ./internal/...

migrate-up:
	cd apps/strait && go run ./cmd/strait migrate up

migrate-down:
	cd apps/strait && go run ./cmd/strait migrate down

migrate-status:
	cd apps/strait && go run ./cmd/strait migrate status

migrate-create:
	cd apps/strait && go run ./cmd/strait migrate create $(NAME)

clean:
	cd apps/strait && go clean -testcache
	docker compose -f apps/strait/docker-compose.yml down -v
	@-kill $$(cat /tmp/stripe-listen.pid) 2>/dev/null; rm -f /tmp/stripe-listen.pid

check: build vet test lint
	@echo "All checks passed"
