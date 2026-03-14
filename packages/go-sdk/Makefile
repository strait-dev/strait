SHELL := /bin/bash

.PHONY: lint typecheck run-all

lint:
	golangci-lint run ./...

typecheck:
	go test ./...

run-all: lint typecheck
