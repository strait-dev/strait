# Strait base runtime image for Ruby jobs.
#
# Uses the official Ruby slim image with Bundler pre-configured.
# The Strait SDK gem is pre-installed to keep user build times short.
#
# Published as: ghcr.io/strait-dev/runtime-ruby:{ruby-version}-{strait-version}
# Latest alias: ghcr.io/strait-dev/runtime-ruby:latest

ARG RUBY_VERSION=3.3

FROM ruby:${RUBY_VERSION}-slim-bookworm

LABEL org.opencontainers.image.source="https://github.com/strait-dev/strait"
LABEL org.opencontainers.image.description="Strait Ruby runtime base image"
LABEL org.opencontainers.image.licenses="Apache-2.0"

# System packages for native gem extensions.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libffi-dev \
    libssl-dev \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Non-root user.
RUN groupadd -r strait && useradd -r -g strait -d /app strait

WORKDIR /app
RUN chown strait:strait /app

# Pre-install the Strait SDK gem.
ARG STRAIT_SDK_VERSION=">= 0"
RUN gem install strait-sdk --version "${STRAIT_SDK_VERSION}" --no-document \
    && gem install bundler --no-document

USER strait

ENTRYPOINT ["ruby"]
