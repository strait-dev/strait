# Strait base runtime image for Python jobs.
#
# This image is the base layer for code-first Python jobs deployed via `strait deploy`.
# BuildKit uses this as FROM and adds the user's code and dependencies on top.
#
# Design goals:
#   - Minimal image size (slim base, no dev tools in prod layer)
#   - Fast layer cache hits for common packages (pip cache mounted during build)
#   - Strait SDK pre-installed so user code can import it without listing it
#
# Published as: ghcr.io/strait-dev/runtime-python:{python-version}-{strait-version}
# Latest alias: ghcr.io/strait-dev/runtime-python:latest

ARG PYTHON_VERSION=3.13

FROM python:${PYTHON_VERSION}-slim-bookworm

# Runtime metadata.
LABEL org.opencontainers.image.source="https://github.com/strait-dev/strait"
LABEL org.opencontainers.image.description="Strait Python runtime base image"
LABEL org.opencontainers.image.licenses="Apache-2.0"

# System packages needed for common native extensions.
# These are installed in the base image to keep user build times short.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libffi-dev \
    libssl-dev \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Non-root user for security. Job code runs as this user.
RUN groupadd -r strait && useradd -r -g strait -d /app strait

WORKDIR /app
RUN chown strait:strait /app

# Pre-install the Strait Python SDK and common dependencies.
# Version is pinned by the builder via --build-arg STRAIT_SDK_VERSION.
# Defaults to empty (latest) so the build succeeds even before the SDK is
# published to PyPI — SDK availability is a best-effort pre-warm.
ARG STRAIT_SDK_VERSION=
RUN if [ -n "${STRAIT_SDK_VERSION}" ]; then \
        pip install --no-cache-dir "strait-sdk==${STRAIT_SDK_VERSION}" httpx pydantic; \
    else \
        pip install --no-cache-dir strait-sdk httpx pydantic || true; \
    fi

USER strait

# The entrypoint is overridden by the generated Dockerfile for each deployment.
# Here we define a sensible default that the Strait SDK replaces.
ENTRYPOINT ["python", "-m", "strait"]
