SHELL := /bin/bash

.PHONY: lint typecheck vet test run-all

lint:
	golangci-lint run ./...

vet:
	go vet ./...

test:
	go test ./...

typecheck: test

run-all: lint vet test
