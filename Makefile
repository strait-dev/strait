.PHONY: help setup dev build test test-race test-int test-load lint vet bench migrate-up migrate-down migrate-status migrate-create clean check

help:
	@echo "Strait Development Commands"
	@echo ""
	@echo "Setup & Dependencies:"
	@echo "  make setup              Download dependencies, tidy modules, install git hooks"
	@echo ""
	@echo "Development:"
	@echo "  make dev                Start Docker dependencies and run app in all mode"
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
	go mod download
	go mod tidy
	lefthook install

dev:
	docker compose up -d
	go run ./cmd/strait --mode all

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

test-int:
	go test -tags integration ./...

test-load:
	go test -tags loadtest -v -timeout=30m ./test/loadtest/...

lint:
	golangci-lint run --timeout=5m ./...

vet:
	go vet ./...

bench:
	go test -bench . -benchmem -run=^$$ ./internal/...

migrate-up:
	go run ./cmd/strait migrate up

migrate-down:
	go run ./cmd/strait migrate down

migrate-status:
	go run ./cmd/strait migrate status

migrate-create:
	go run ./cmd/strait migrate create $(NAME)

clean:
	go clean -testcache
	docker compose down -v

check: build vet test lint
	@echo "All checks passed"
