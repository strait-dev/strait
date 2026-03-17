.PHONY: build test lint vet dev preflight deploy deploy-staging

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

preflight: vet lint build test

deploy: preflight
	fly deploy -a strait

deploy-staging: preflight
	fly deploy -a strait -c fly.toml -e DOPPLER_CONFIG=stg
