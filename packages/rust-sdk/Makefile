.PHONY: build test lint run-all

build:
	cargo build

test:
	cargo test

lint:
	cargo clippy -- -D warnings

run-all: lint test
