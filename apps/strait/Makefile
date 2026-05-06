.PHONY: build test lint vet preflight deploy deploy-staging
.PHONY: dev dev-up dev-down dev-status dev-logs dev-clean
.PHONY: loadtest-server loadtest-quick loadtest-throughput
.PHONY: loadtest-concurrency loadtest-endurance loadtest-chaos loadtest-errors
.PHONY: loadtest-all loadtest-report loadtest-unit loadtest-flush
.PHONY: proto

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

lint:
	golangci-lint run --timeout=10m ./...

vet:
	go vet ./...

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
#   make dev          Start dependencies and run Strait in all mode
#   make dev-up       Start Postgres, Redis, and Sequin
#   make dev-status   Show Docker Compose service status
#   make dev-logs     Show Docker Compose service logs
#   make dev-clean    Stop dependencies and remove local volumes
#   make dev-down     Stop dependencies

DEV_ENV = \
	DATABASE_URL="postgres://strait:strait@localhost:15432/strait?sslmode=disable" \
	REDIS_URL="redis://localhost:16379" \
	INTERNAL_SECRET="local-dev-internal-secret-change-me" \
	JWT_SIGNING_KEY="local-dev-jwt-signing-key-change-me-32chars" \
	ALLOW_PRIVATE_ENDPOINTS=true \
	CORS_ALLOWED_ORIGINS="*" \
	PORT=8080

dev: dev-up
	$(DEV_ENV) go run ./cmd/strait --mode all

dev-up:
	@echo "Starting Strait infrastructure..."
	@docker compose up -d
	@echo ""
	@echo "Infrastructure ready:"
	@echo "  Postgres:    localhost:15432 (strait/strait)"
	@echo "  Redis:       localhost:16379"
	@echo "  Sequin:      localhost:7376"
	@echo ""
	@echo "Run Strait:"
	@echo "  make dev"

dev-down:
	@docker compose down
	@echo "All infrastructure stopped."

dev-status:
	@echo "=== Docker Compose ==="
	@docker compose ps 2>/dev/null || echo "Not running. Run: make dev-up"

dev-logs:
	@docker compose logs --tail=200

dev-clean:
	@docker compose down -v
	@echo "Local development infrastructure cleaned up."

# ──────────────────────────────────────────────
# Load Testing
# ──────────────────────────────────────────────

LOADTEST_PORT ?= 7676

LOADTEST_ENV = \
	DATABASE_URL="postgres://strait:strait@localhost:15432/strait?sslmode=disable" \
	REDIS_URL="redis://localhost:16379" \
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
	REDIS_URL="redis://localhost:16379"

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
