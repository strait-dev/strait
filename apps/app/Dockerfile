# Strait Dashboard (TanStack Start)
#
# Build from the monorepo root:
#   docker build -t ghcr.io/strait-dev/strait-app -f apps/app/Dockerfile .
#
# Run:
#   docker run -p 3000:3000 \
#     -e STRAIT_API_URL=http://host.docker.internal:8080 \
#     -e INTERNAL_SECRET=your-secret \
#     -e AUTH_DATABASE_URL=postgres://strait:strait@host.docker.internal:5432/strait \
#     -e BETTER_AUTH_URL=http://localhost:3000 \
#     -e BETTER_AUTH_SECRET=your-auth-secret \
#     ghcr.io/strait-dev/strait-app

# -- Install stage --
FROM oven/bun:1 AS install

WORKDIR /app

# Copy all workspace package.json files for lockfile resolution.
COPY package.json bun.lock ./
COPY apps/app/package.json apps/app/package.json
COPY apps/docs/package.json apps/docs/package.json
COPY apps/strait/package.json apps/strait/package.json
COPY apps/website/package.json apps/website/package.json
COPY packages/billing/package.json packages/billing/package.json
COPY packages/config/package.json packages/config/package.json
COPY packages/transactional/package.json packages/transactional/package.json
COPY packages/typescript-sdk/package.json packages/typescript-sdk/package.json
COPY packages/ui/package.json packages/ui/package.json
COPY packages/utils/package.json packages/utils/package.json

RUN bun install --frozen-lockfile

# -- Build stage --
FROM oven/bun:1 AS build

WORKDIR /app

# Copy installed dependencies (root + workspace node_modules).
COPY --from=install /app/ ./

# Copy all source (workspace packages needed for build).
COPY apps/app/ apps/app/
COPY packages/ packages/

# Build with node-server preset for Docker (vite.config.ts reads NITRO_PRESET).
ENV NITRO_PRESET=node-server
RUN cd apps/app && bun run build

# -- Runtime stage --
FROM node:22-alpine

RUN apk add --no-cache curl

WORKDIR /app

COPY --from=build /app/apps/app/.output ./.output

ENV PORT=3000
ENV HOST=0.0.0.0

EXPOSE 3000

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD ["curl", "-sf", "http://localhost:3000/", "-o", "/dev/null"]

CMD ["node", ".output/server/index.mjs"]
