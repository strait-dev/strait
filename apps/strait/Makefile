.PHONY: build test lint vet preflight deploy deploy-staging
.PHONY: dev dev-up dev-down dev-http dev-docker dev-status dev-logs dev-clean
.PHONY: loadtest-up loadtest-down loadtest-server loadtest-quick loadtest-throughput
.PHONY: loadtest-concurrency loadtest-endurance loadtest-chaos loadtest-errors
.PHONY: loadtest-all loadtest-report loadtest-unit loadtest-flush
.PHONY: test-k8s-unit test-k8s-integration

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

preflight: vet lint build test

deploy: preflight
	fly deploy -a strait

deploy-staging: preflight
	fly deploy -a strait -c fly.toml -e DOPPLER_CONFIG=stg

# ──────────────────────────────────────────────
# Local Development
# ──────────────────────────────────────────────
#
# Quick start (one command, full managed mode):
#
#   make dev
#
# This starts Postgres, Redis, Sequin, a local Kubernetes cluster (kind),
# and the Strait server with managed mode enabled. Job containers run on
# the kind cluster on your machine.
#
# Other modes:
#   make dev-http     Start Strait in HTTP mode (no container runtime)
#   make dev-docker   Start Strait with Docker runtime (simpler, no K8s)
#   make dev-status   Show status of all infrastructure
#   make dev-logs     Show logs of the most recent K8s job
#   make dev-clean    Delete all K8s jobs and pods
#   make dev-down     Stop everything (Postgres, Redis, Sequin, kind)

DEV_ENV = \
	DATABASE_URL="postgres://strait:strait@localhost:5432/strait?sslmode=disable" \
	REDIS_URL="redis://localhost:6379" \
	INTERNAL_SECRET="local-dev-internal-secret-change-me" \
	JWT_SIGNING_KEY="local-dev-jwt-signing-key-change-me-32chars" \
	ALLOW_PRIVATE_ENDPOINTS=true \
	CORS_ALLOWED_ORIGINS="*" \
	PORT=8080

dev: dev-up
	$(DEV_ENV) \
	COMPUTE_RUNTIME=k8s \
	K8S_KUBECONFIG="$$HOME/.kube/config" \
	K8S_NAMESPACE=default \
	EXTERNAL_API_URL="http://host.docker.internal:8080" \
	ALLOWED_IMAGE_REGISTRIES="docker.io,ghcr.io,registry.k8s.io" \
	go run ./cmd/strait

dev-up:
	@echo "Starting Strait infrastructure..."
	@docker compose up -d
	@# Create kind cluster named "strait" if it doesn't exist.
	@kind get clusters 2>/dev/null | grep -q "^strait$$" || ( \
		echo "Creating Kubernetes cluster..." && \
		kind create cluster --name strait && \
		kubectl apply -f k8s/priority-classes.yaml --context kind-strait && \
		kubectl apply -f k8s/service-account.yaml --context kind-strait && \
		kubectl apply -f k8s/resource-quota.yaml --context kind-strait \
	)
	@echo ""
	@echo "Infrastructure ready:"
	@echo "  Postgres:    localhost:5432 (strait/strait)"
	@echo "  Redis:       localhost:6379"
	@echo "  Sequin:      localhost:7376"
	@echo "  Kubernetes:  kind-strait (kubectl --context kind-strait)"
	@echo ""
	@echo "Run Strait:"
	@echo "  make dev           Managed mode (K8s)"
	@echo "  make dev-http      HTTP mode (no containers)"
	@echo "  make dev-docker    Managed mode (Docker)"

dev-down:
	@docker compose down
	@kind delete cluster --name strait 2>/dev/null || true
	@echo "All infrastructure stopped."

dev-http:
	$(DEV_ENV) COMPUTE_RUNTIME=none go run ./cmd/strait

dev-docker:
	$(DEV_ENV) \
	COMPUTE_RUNTIME=docker \
	EXTERNAL_API_URL="http://host.docker.internal:8080" \
	ALLOWED_IMAGE_REGISTRIES="docker.io,ghcr.io" \
	go run ./cmd/strait

dev-status:
	@echo "=== Docker Compose ==="
	@docker compose ps 2>/dev/null || echo "Not running. Run: make dev-up"
	@echo ""
	@echo "=== Kubernetes ==="
	@kubectl cluster-info --context kind-strait 2>/dev/null || echo "No cluster. Run: make dev-up"
	@echo ""
	@echo "=== K8s Jobs ==="
	@kubectl get jobs --context kind-strait -n default 2>/dev/null || true
	@echo ""
	@echo "=== K8s Pods ==="
	@kubectl get pods --context kind-strait -n default 2>/dev/null || true

dev-logs:
	@kubectl get pods --context kind-strait -n default --sort-by=.metadata.creationTimestamp -o name 2>/dev/null | tail -1 | xargs -I{} kubectl logs --context kind-strait -n default {} 2>/dev/null || echo "No pods found."

dev-clean:
	@kubectl delete jobs --all --context kind-strait -n default 2>/dev/null || true
	@kubectl delete pods --all --context kind-strait -n default 2>/dev/null || true
	@echo "All jobs and pods cleaned up."

# ──────────────────────────────────────────────
# Tests
# ──────────────────────────────────────────────

test-k8s-unit:
	go test -race -short ./internal/compute/... -v

test-k8s-integration:
	go test -race ./internal/compute/ -run Integration -v -timeout 5m

test-k8s-stress:
	go test -race ./internal/compute/ -run "TestStress_|TestK8sStress_" -v -timeout 30m

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
