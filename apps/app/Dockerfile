# syntax=docker/dockerfile:1.7
#
# Strait App — Node.js runtime image for self-host.
#
# Two stages:
#   1. builder: Bun image installs the full workspace and runs the Vite
#      build with BUILD_TARGET=node, producing a self-contained SSR
#      bundle at apps/app/dist/server/server.js and static assets at
#      apps/app/dist/client/.
#   2. runtime: node:22-alpine serves the built bundle via the Node
#      HTTP adapter at apps/app/scripts/node-server.mjs. No runtime
#      node_modules are required — the adapter uses only node: built-ins
#      and the SSR bundle inlines all dependencies (ssr.noExternal in
#      vite.config.ts).
#
# Build context is the repo root (not apps/app) because the TanStack
# Start app depends on workspace packages @strait/transactional and
# @strait/ui. The root .dockerignore already excludes everything not
# needed.
#
# Base image digests are deliberately unpinned on this commit. Dependabot's
# docker ecosystem will pin both images on its first run, matching the
# convention used in apps/strait/Dockerfile.

FROM --platform=$BUILDPLATFORM oven/bun:1.3.11-alpine AS builder
WORKDIR /repo

# Copy the lockfile and every workspace manifest first so `bun install`
# gets its own layer that only invalidates when dependencies change.
COPY package.json bun.lock turbo.json ./
COPY apps/app/package.json apps/app/
COPY packages/transactional/package.json packages/transactional/
COPY packages/ui/package.json packages/ui/

RUN bun install --frozen-lockfile

# Source. Excluded paths are controlled by the root .dockerignore.
COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
ENV VITE_APP_VERSION=$VERSION \
    VITE_APP_COMMIT=$COMMIT \
    VITE_APP_DATE=$DATE

RUN cd apps/app && BUILD_TARGET=node bun run build

FROM node:22-alpine AS runtime
WORKDIR /app

RUN addgroup -S strait && adduser -S -G strait strait

COPY --from=builder --chown=strait:strait /repo/apps/app/dist ./dist
COPY --from=builder --chown=strait:strait /repo/apps/app/scripts/node-server.mjs ./scripts/node-server.mjs

USER strait:strait

ENV NODE_ENV=production \
    PORT=3000 \
    HOST=0.0.0.0

EXPOSE 3000

# `/` redirects to `/login` (307) for unauthenticated users, so probe
# `/login` directly in spider mode — a cheap HEAD that doesn't download
# the body. Returns non-zero on network error or non-2xx response.
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:3000/login || exit 1

CMD ["node", "./scripts/node-server.mjs"]
