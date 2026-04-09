# Strait base runtime image for Rust jobs.
#
# Two-stage design: the builder stage compiles the user's crate, the runtime
# stage is a minimal Debian image that contains only the compiled binary.
# This keeps the final image under 50MB for most jobs.
#
# Stage 1 (builder): used by BuildKit during `strait deploy` to compile user code.
# Stage 2 (runtime): the final image that runs on each job invocation.
#
# Published as: ghcr.io/strait-dev/runtime-rust:{rust-version}-{strait-version}
# Latest alias: ghcr.io/strait-dev/runtime-rust:latest

ARG RUST_VERSION=1.83
ARG DEBIAN_VERSION=bookworm

# --- Builder stage ---
FROM rust:${RUST_VERSION}-slim-${DEBIAN_VERSION} AS builder

LABEL org.opencontainers.image.source="https://github.com/strait-dev/strait"
LABEL org.opencontainers.image.description="Strait Rust runtime builder image"

# Build tools and SSL for crates that use native TLS.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pkg-config \
    libssl-dev \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Pre-warm cargo registry cache with the Strait SDK dependency.
# When user code depends on strait-sdk, this layer is already cached.
ARG STRAIT_SDK_VERSION="*"
RUN cargo install --locked \
    strait-sdk@${STRAIT_SDK_VERSION} 2>/dev/null || true

WORKDIR /build

# --- Runtime stage ---
FROM debian:${DEBIAN_VERSION}-slim AS runtime

LABEL org.opencontainers.image.source="https://github.com/strait-dev/strait"
LABEL org.opencontainers.image.description="Strait Rust runtime base image"
LABEL org.opencontainers.image.licenses="Apache-2.0"

# Minimal runtime deps: CA certs for TLS, libssl for native-TLS crates.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libssl3 \
    && rm -rf /var/lib/apt/lists/*

# Non-root user.
RUN groupadd -r strait && useradd -r -g strait -d /app strait

WORKDIR /app
RUN chown strait:strait /app

USER strait

# The final entrypoint is the compiled binary, copied from the builder stage
# by the per-deployment generated Dockerfile.
ENTRYPOINT ["/app/handler"]
