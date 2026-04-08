# Strait base runtime image for Go jobs.
#
# Two-stage design mirroring the Rust Dockerfile: builder stage compiles the
# user's module, runtime stage is a minimal scratch-based image.
#
# The Go builder pre-downloads the Strait SDK module so the module cache is
# warm for user builds that import it.
#
# Published as: ghcr.io/strait-dev/runtime-go:{go-version}-{strait-version}
# Latest alias: ghcr.io/strait-dev/runtime-go:latest

ARG GO_VERSION=1.26

# --- Builder stage ---
FROM golang:${GO_VERSION}-bookworm AS builder

LABEL org.opencontainers.image.source="https://github.com/strait-dev/strait"
LABEL org.opencontainers.image.description="Strait Go runtime builder image"

# Build tools.
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Pre-warm module cache with the Strait SDK.
ARG STRAIT_SDK_VERSION=latest
RUN mkdir -p /tmp/warmup && cd /tmp/warmup \
    && go mod init warmup \
    && go get github.com/strait-dev/sdk-go@${STRAIT_SDK_VERSION} 2>/dev/null || true \
    && rm -rf /tmp/warmup

WORKDIR /build

# --- Runtime stage ---
FROM gcr.io/distroless/static-debian12 AS runtime

LABEL org.opencontainers.image.source="https://github.com/strait-dev/strait"
LABEL org.opencontainers.image.description="Strait Go runtime base image"
LABEL org.opencontainers.image.licenses="Apache-2.0"

# distroless runs as nonroot (uid 65532) by default.
USER nonroot:nonroot

WORKDIR /app

# The final entrypoint is the compiled binary, copied from the builder stage
# by the per-deployment generated Dockerfile.
ENTRYPOINT ["/app/handler"]
