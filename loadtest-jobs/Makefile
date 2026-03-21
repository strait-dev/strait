.PHONY: build build-python build-typescript build-go build-errors clean

IMAGE_PREFIX ?= strait-loadtest

build: build-python build-typescript build-go build-errors

build-python:
	docker build -f Dockerfile.python -t $(IMAGE_PREFIX)-python .

build-typescript:
	docker build -f Dockerfile.typescript -t $(IMAGE_PREFIX)-ts .

build-go:
	docker build -f Dockerfile.go -t $(IMAGE_PREFIX)-go .

build-errors:
	docker build -f Dockerfile.errors -t $(IMAGE_PREFIX)-errors .

clean:
	docker rmi $(IMAGE_PREFIX)-python $(IMAGE_PREFIX)-ts $(IMAGE_PREFIX)-go $(IMAGE_PREFIX)-errors 2>/dev/null || true
