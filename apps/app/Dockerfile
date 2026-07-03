# syntax=docker/dockerfile:1.7
#
# Strait App — Node.js runtime image for self-host.
#
# Two stages:
#   1. builder: Bun image installs the full workspace and runs the Vite
#      build with the Nitro node-server preset, producing
#      apps/app/.output/server/index.mjs and static assets under
#      apps/app/.output/public/.
#   2. runtime: gcr.io/distroless/nodejs22-debian12:nonroot runs the
#      Nitro Node server. Distroless ships only Node,
#      its dynamic deps, ca-certificates, tzdata, and /etc/passwd with
#      a nonroot user — no shell, no package manager, no npm (which
#      previously dragged in a vulnerable picomatch), no Alpine apk
#      base that carried recurring libcrypto/libssl distro-lag CVEs.
#      This matches the security posture of apps/strait/Dockerfile
#      (see its runtime-stage comment at line 49).
#
# Build context is the repo root (not apps/app) because the TanStack
# Start app depends on workspace package @strait/transactional.
# Copy only the app and package source needed for the build
# so untracked local secret files in the repo root are not copied into
# builder layers.

FROM --platform=$BUILDPLATFORM oven/bun:1.3.14-alpine AS builder
WORKDIR /repo

# Copy the lockfile and every workspace manifest first so `bun install`
# gets its own layer that only invalidates when dependencies change.
# Every package.json listed in the root workspaces glob must be copied —
# bun install --frozen-lockfile fails if any are missing.
COPY package.json bun.lock turbo.json ./
COPY apps/app/package.json apps/app/
COPY apps/docs/package.json apps/docs/
COPY apps/strait/package.json apps/strait/
COPY packages/billing/package.json packages/billing/
COPY packages/config/package.json packages/config/
COPY packages/transactional/package.json packages/transactional/

RUN bun install --frozen-lockfile

# Source. Keep this explicit so ignored local secrets never enter the build stage.
COPY apps/app/instrument.server.ts apps/app/knip.json apps/app/tsconfig.json apps/app/vite.config.ts apps/app/vitest.config.ts apps/app/
COPY apps/app/public apps/app/public
COPY apps/app/scripts apps/app/scripts
COPY apps/app/src apps/app/src
COPY packages packages

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
ENV VITE_APP_VERSION=$VERSION \
    VITE_APP_COMMIT=$COMMIT \
    VITE_APP_DATE=$DATE \
    VITE_STRAIT_EDITION=community

RUN cd apps/app && bun run build:node && bun run build:migrate && find .output -name '*.map' -type f -delete

FROM gcr.io/distroless/nodejs22-debian12:nonroot@sha256:13593b7570658e8477de39e2f4a1dd25db2f836d68a0ba771251572d23bb4f8e AS runtime
WORKDIR /app

# `nonroot` (UID 65532) is provided by the distroless image — no
# addgroup/adduser needed. COPY --chown references the name directly.
COPY --from=builder --chown=nonroot:nonroot /repo/apps/app/.output ./.output

USER nonroot:nonroot

ENV NODE_ENV=production \
    PORT=3000 \
    HOST=0.0.0.0

EXPOSE 3000

# Distroless has no shell or wget, so healthcheck uses Node's built-in
# fetch to probe /login. `/` returns 307 to /login; probing /login
# directly accepts the 200 response. Fails the check on any non-2xx,
# network error, or exception.
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=3 \
    CMD ["/nodejs/bin/node", "-e", "fetch('http://127.0.0.1:3000/login').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"]

# Distroless sets ENTRYPOINT to the node binary, so CMD is the argv.
CMD [".output/server/index.mjs"]
