# Strait base runtime image for TypeScript/JavaScript jobs.
#
# Uses Bun for fast package installs and TypeScript execution without a
# separate compilation step. Node.js is also available for compatibility.
#
# Published as: ghcr.io/strait-dev/runtime-typescript:{bun-version}-{strait-version}
# Latest alias: ghcr.io/strait-dev/runtime-typescript:latest

ARG BUN_VERSION=1.2

FROM oven/bun:${BUN_VERSION}-slim

LABEL org.opencontainers.image.source="https://github.com/strait-dev/strait"
LABEL org.opencontainers.image.description="Strait TypeScript/JavaScript runtime base image"
LABEL org.opencontainers.image.licenses="Apache-2.0"

# System packages for native addons.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    python3 \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Non-root user.
RUN groupadd -r strait && useradd -r -g strait -d /app strait

WORKDIR /app
RUN chown strait:strait /app

# Pre-install the Strait SDK so user bundles don't need to fetch it.
ARG STRAIT_SDK_VERSION=latest
RUN bun add @strait/sdk@${STRAIT_SDK_VERSION:-latest} --global

USER strait

ENTRYPOINT ["bun", "run"]
