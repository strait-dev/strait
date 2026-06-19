.PHONY: build test lint vet mutation-test preflight deploy deploy-staging
.PHONY: test-integration-fast test-integration-clean
.PHONY: dev dev-up dev-down dev-status dev-logs dev-clean
.PHONY: loadtest-server loadtest-quick loadtest-throughput
.PHONY: loadtest-concurrency loadtest-endurance loadtest-chaos loadtest-errors
.PHONY: loadtest-all loadtest-report loadtest-unit loadtest-flush
.PHONY: proto

DEV_COMPOSE = docker compose -f ../../docker-compose.base.yml -f docker-compose.yml

proto: ## Regenerate gRPC Go code from .proto sources (requires buf, protoc-gen-go, protoc-gen-go-grpc)
	buf generate
	cp internal/api/grpc/proto/worker/v1/worker.pb.go internal/api/grpc/proto/workerv1/worker.pb.go
	cp internal/api/grpc/proto/worker/v1/worker_grpc.pb.go internal/api/grpc/proto/workerv1/worker_grpc.pb.go
	rm -rf internal/api/grpc/proto/worker

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race -timeout=60s ./...

test-integration:
	go test -race -tags=integration -timeout=20m ./internal/store ./internal/queue ./internal/pubsub ./internal/e2e

test-integration-fast:
	./scripts/test-integration-fast.sh --all

test-integration-clean:
	./scripts/test-integration-clean.sh

lint:
	./scripts/lint-go.sh

vet:
	go vet ./...

mutation-test:
	@if [ -z "$(PKG)" ]; then \
		echo "Usage: make mutation-test PKG=./internal/errors"; \
		exit 2; \
	fi
	./scripts/mutation-test.sh $(PKG)

vulncheck:
	govulncheck ./...

preflight: vet lint build test

deploy: preflight ## Deploy using the infra repo
	@echo "Deploy via the infra repo: https://github.com/strait-dev/infra"
	@echo "Trigger the appropriate deploy workflow from that repo."

# ──────────────────────────────────────────────
# Local Development
# ──────────────────────────────────────────────
#
# Quick start (one command, local orchestration stack):
#
#   make dev
#
# This starts Postgres, Redis, Sequin, and the Strait server. Strait is
# orchestration-only: job code runs in customer HTTP endpoints or connected
# worker-mode clients.
#
# Commands:
#   make dev          Start the full Strait stack
#   make dev-up       Start Postgres, Redis, Sequin, and Strait
#   make dev-status   Show Docker Compose service status
#   make dev-logs     Show Docker Compose service logs
#   make dev-clean    Stop dependencies and remove local volumes
#   make dev-down     Stop dependencies

DEV_ENV = \
	DATABASE_URL="postgres://strait:strait@localhost:15432/strait?sslmode=disable" \
	REDIS_URL="redis://localhost:16379" \
	SEQUIN_BASE_URL="http://localhost:7376" \
	SEQUIN_CONSUMER_NAME="strait-cdc" \
	INTERNAL_SECRET="local-dev-internal-secret-change-me" \
	JWT_SIGNING_KEY="local-dev-jwt-signing-key-change-me-32chars" \
	ALLOW_PRIVATE_ENDPOINTS=true \
	CORS_ALLOWED_ORIGINS="*" \
	PORT=8080

dev: dev-up

dev-up:
	@echo "Starting Strait stack..."
	@$(DEV_COMPOSE) --profile core up --build --wait -d
	@echo ""
	@echo "Strait stack ready:"
	@echo "  API:      http://localhost:8080"
	@echo "  Postgres: localhost:15432 (strait/strait)"
	@echo "  Redis:    localhost:16379"
	@echo "  Sequin:   http://localhost:7376"

dev-down:
	@$(DEV_COMPOSE) --profile core down
	@echo "All infrastructure stopped."

dev-status:
	@echo "=== Docker Compose ==="
	@$(DEV_COMPOSE) --profile core ps 2>/dev/null || echo "Not running. Run: make dev-up"

dev-logs:
	@$(DEV_COMPOSE) --profile core logs --tail=200

dev-clean:
	@$(DEV_COMPOSE) --profile core --profile debug --profile observability down -v
	@echo "Local development infrastructure cleaned up."

# ──────────────────────────────────────────────
# Load Testing
# ──────────────────────────────────────────────

LOADTEST_PORT ?= 7676

LOADTEST_ENV = \
	DATABASE_URL="postgres://strait:strait@localhost:15432/strait?sslmode=disable" \
	REDIS_URL="redis://localhost:16379" \
	SEQUIN_BASE_URL="http://localhost:7376" \
	SEQUIN_CONSUMER_NAME="strait-cdc" \
	INTERNAL_SECRET="loadtest-secret-that-is-long-enough" \
	JWT_SIGNING_KEY="loadtest-jwt-signing-key-at-least-32-chars" \
	ALLOW_PRIVATE_ENDPOINTS=true \
	RATE_LIMIT_REQUESTS=100000 \
	TRIGGER_RATE_LIMIT_REQUESTS=100000 \
	PORT=$(LOADTEST_PORT)

LOADTEST_RUN = \
	LOADTEST_STRAIT_URL=http://localhost:$(LOADTEST_PORT) \
	INTERNAL_SECRET="loadtest-secret-that-is-long-enough" \
	DATABASE_URL="postgres://strait:strait@localhost:15432/strait?sslmode=disable" \
	REDIS_URL="redis://localhost:16379" \
	SEQUIN_BASE_URL="http://localhost:7376" \
	SEQUIN_CONSUMER_NAME="strait-cdc"

loadtest-server:
	$(LOADTEST_ENV) go run ./cmd/strait

loadtest-quick:
	$(LOADTEST_RUN) LOADTEST_QUICK=true go test -tags=loadtest -run TestQuickValidation -timeout 15m -v ./internal/loadtest/...

loadtest-throughput:
	$(LOADTEST_RUN) go test -tags=loadtest -run TestThroughputCeiling -timeout 2h -v ./internal/loadtest/...

loadtest-concurrency:
	$(LOADTEST_RUN) go test -tags=loadtest -run TestConcurrencyCeiling -timeout 1h -v ./internal/loadtest/...

loadtest-endurance:
	$(LOADTEST_RUN) LOADTEST_DURATION=24h go test -tags=loadtest -run TestEndurance -timeout 26h -v ./internal/loadtest/...

loadtest-chaos:
	$(LOADTEST_RUN) go test -tags=loadtest -run TestChaosAll -timeout 4h -v ./internal/loadtest/...

loadtest-errors:
	$(LOADTEST_RUN) go test -tags=loadtest -run TestErrorScenarios -timeout 1h -v ./internal/loadtest/...

loadtest-all: loadtest-quick loadtest-throughput loadtest-concurrency

loadtest-flush:
	@echo "Flushing queued and running load test runs..."
	@docker exec $$(docker ps -q --filter "ancestor=postgres:18-alpine" | head -1) psql -U strait -d strait -c "DELETE FROM job_runs WHERE status IN ('queued', 'running') AND project_id = 'loadtest-project';"
	@echo "Done. Queue is clear."

loadtest-report:
	go run -tags=loadtest ./internal/loadtest/cmd/report -input internal/loadtest/loadtest-results/ -html loadtest-results/report.html -json loadtest-results/report.json

loadtest-unit:
	go test -tags=loadtest -run "TestReport|TestMetrics|TestLatency" -timeout 30s -v ./internal/loadtest/...
