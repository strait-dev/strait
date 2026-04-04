.PHONY: build test lint vet dev preflight deploy deploy-staging
.PHONY: dev-up dev-down dev-http dev-managed dev-setup
.PHONY: loadtest-up loadtest-down loadtest-server loadtest-quick loadtest-throughput
.PHONY: loadtest-concurrency loadtest-endurance loadtest-chaos loadtest-errors
.PHONY: loadtest-all loadtest-report loadtest-unit loadtest-flush
.PHONY: dev-k8s dev-k8s-down dev-k8s-managed dev-k8s-http dev-k8s-status
.PHONY: dev-k8s-logs dev-k8s-clean test-k8s-unit test-k8s-integration

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race -timeout=60s ./...

test-integration:
	go test -race -tags=integration -timeout=20m ./internal/store ./internal/queue ./internal/pubsub ./internal/e2e

lint:
	golangci-lint run --timeout=5m ./...

vet:
	go vet ./...

vulncheck:
	govulncheck ./...

dev:
	go run ./cmd/strait

# ───���──────────────────────────────────────────
# Local Development (Contributors)
# ──────────────────────────────────────────────
# Quick start:
#   make dev-up       # Start Postgres + Redis
#   make dev-http     # Start Strait in HTTP mode (no container runtime)
#   make dev-managed  # Start Strait in managed mode (Docker runtime)
#
# For managed mode, Docker must be running. Job containers execute via
# the local Docker daemon — no cloud services or K8s needed.

DEV_ENV = \
	DATABASE_URL="postgres://strait:strait@localhost:5432/strait?sslmode=disable" \
	REDIS_URL="redis://localhost:6379" \
	INTERNAL_SECRET="local-dev-internal-secret-change-me" \
	JWT_SIGNING_KEY="local-dev-jwt-signing-key-change-me-32chars" \
	ALLOW_PRIVATE_ENDPOINTS=true \
	CORS_ALLOWED_ORIGINS="*" \
	PORT=8080

dev-up:
	docker compose up -d postgres redis
	@echo ""
	@echo "Postgres: localhost:5432 (strait/strait)"
	@echo "Redis:    localhost:6379"
	@echo ""
	@echo "Next steps:"
	@echo "  make dev-http      # HTTP mode (no container runtime)"
	@echo "  make dev-managed   # Managed mode (Docker runtime)"

dev-down:
	docker compose down

dev-http:
	$(DEV_ENV) COMPUTE_RUNTIME=none go run ./cmd/strait

dev-managed:
	$(DEV_ENV) \
	COMPUTE_RUNTIME=docker \
	EXTERNAL_API_URL="http://host.docker.internal:8080" \
	ALLOWED_IMAGE_REGISTRIES="docker.io,ghcr.io" \
	go run ./cmd/strait

dev-setup:
	@echo "Setting up Strait for local development..."
	docker compose up -d postgres redis
	@echo "Waiting for Postgres..."
	@sleep 3
	$(DEV_ENV) COMPUTE_RUNTIME=none go run ./cmd/strait &
	@sleep 5
	@kill %1 2>/dev/null || true
	@echo ""
	@echo "Setup complete. Database migrations applied."
	@echo ""
	@echo "Run Strait:"
	@echo "  make dev-http      # HTTP mode (no container runtime)"
	@echo "  make dev-managed   # Managed mode (runs containers via Docker)"
	@echo ""
	@echo "Managed mode test:"
	@echo "  1. make dev-managed"
	@echo "  2. Create a job with execution_mode: managed and image_uri: alpine:3.19"
	@echo "  3. Trigger the job — it runs as a Docker container on your machine"

preflight: vet lint build test

deploy: preflight
	fly deploy -a strait

deploy-staging: preflight
	fly deploy -a strait -c fly.toml -e DOPPLER_CONFIG=stg

# ──────────────────────────────────────────────
# Load Testing
# ──────────────────────────────────────────────

LOADTEST_PORT ?= 7676

LOADTEST_ENV = \
	DATABASE_URL="postgres://strait:strait@localhost:5432/strait?sslmode=disable" \
	REDIS_URL="redis://localhost:6379" \
	INTERNAL_SECRET="loadtest-secret-that-is-long-enough" \
	JWT_SIGNING_KEY="loadtest-jwt-signing-key-at-least-32-chars" \
	ALLOW_PRIVATE_ENDPOINTS=true \
	RATE_LIMIT_REQUESTS=100000 \
	TRIGGER_RATE_LIMIT_REQUESTS=100000 \
	PORT=$(LOADTEST_PORT)

LOADTEST_RUN = \
	LOADTEST_STRAIT_URL=http://localhost:$(LOADTEST_PORT) \
	INTERNAL_SECRET="loadtest-secret-that-is-long-enough" \
	DATABASE_URL="postgres://strait:strait@localhost:5432/strait?sslmode=disable" \
	REDIS_URL="redis://localhost:6379"

loadtest-up:
	docker compose -f docker-compose.loadtest.yml up -d
	@echo "Postgres:   localhost:5432"
	@echo "Redis:      localhost:6379"
	@echo "Prometheus: http://localhost:9090"
	@echo "Grafana:    http://localhost:3001 (admin/admin)"

loadtest-down:
	docker compose -f docker-compose.loadtest.yml down

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

# ──────────────────────────────────────────────
# K8s Local Development (kind)
# ──────────────────────────────────────────────

K8S_DEV_ENV = \
	COMPUTE_RUNTIME=k8s \
	K8S_NAMESPACE=default \
	DATABASE_URL="postgres://strait:strait@localhost:5432/strait?sslmode=disable" \
	REDIS_URL="redis://localhost:6379" \
	INTERNAL_SECRET="dev-secret-that-is-long-enough-32chars" \
	JWT_SIGNING_KEY="dev-jwt-signing-key-at-least-32-chars"

dev-k8s:
	kind create cluster --name strait-dev 2>/dev/null || true
	kubectl apply -f k8s/priority-classes.yaml --context kind-strait-dev
	kubectl apply -f k8s/service-account.yaml --context kind-strait-dev
	@echo ""
	@echo "K8s dev cluster ready."
	@echo "  Run managed mode:  make dev-k8s-managed"
	@echo "  Run HTTP mode:     make dev-k8s-http"
	@echo "  Check status:      make dev-k8s-status"
	@echo "  View latest logs:  make dev-k8s-logs"
	@echo "  Clean up jobs:     make dev-k8s-clean"

dev-k8s-down:
	kind delete cluster --name strait-dev

dev-k8s-managed:
	$(K8S_DEV_ENV) go run ./cmd/strait

dev-k8s-http:
	$(K8S_DEV_ENV) ALLOW_PRIVATE_ENDPOINTS=true go run ./cmd/strait

dev-k8s-status:
	@echo "=== Kind Cluster ==="
	@kubectl cluster-info --context kind-strait-dev 2>/dev/null || echo "No kind cluster running. Run: make dev-k8s"
	@echo ""
	@echo "=== K8s Jobs ==="
	@kubectl get jobs --context kind-strait-dev -n default 2>/dev/null || true
	@echo ""
	@echo "=== K8s Pods ==="
	@kubectl get pods --context kind-strait-dev -n default 2>/dev/null || true

dev-k8s-logs:
	@kubectl get pods --context kind-strait-dev -n default --sort-by=.metadata.creationTimestamp -o name 2>/dev/null | tail -1 | xargs -I{} kubectl logs --context kind-strait-dev -n default {} 2>/dev/null || echo "No pods found."

dev-k8s-clean:
	@kubectl delete jobs --all --context kind-strait-dev -n default 2>/dev/null || true
	@kubectl delete pods --all --context kind-strait-dev -n default 2>/dev/null || true
	@echo "All jobs and pods cleaned up."

test-k8s-unit:
	go test -race -short ./internal/compute/... -v

test-k8s-integration:
	go test -race ./internal/compute/ -run Integration -v -timeout 5m
