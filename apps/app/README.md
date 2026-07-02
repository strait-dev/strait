# Strait App

Job orchestration management UI built with TanStack Start. Create and monitor jobs, track runs in real time, manage workflows and schedules, configure webhooks, and view logs.

## Source Of Truth

| Area | Source |
|---|---|
| Routes | `src/routes/` |
| API client and server functions | `src/lib/api-client.server.ts`, `src/hooks/api/` |
| Auth | `src/lib/auth.server.ts` |
| Status labels and UI constants | `src/lib/status/` |
| E2E tests | `e2e/` |
| Customer docs | `../docs/` |

## Quick Start (local development)

```bash
# Install dependencies (from repo root)
bun install

# Start development server
cd apps/app
bun dev
```

The app runs at `http://localhost:5173`.

## Deploy

The dashboard is deployable on multiple hosting platforms. Docker is the
portable self-host path and works on any host that can run a container, including
Render, Railway, Fly.io, Cloud Run, VPS hosts, and k8s. Cloudflare Workers and
Vercel are explicit hosted targets because they need platform-specific output.

| Target | Command | Output | Use case |
|---|---|---|---|
| Docker / Node | `bun run build:node` | `.output/server/index.mjs` | Self-hosting and Docker-capable platforms |
| Vercel | `bun run build:vercel` | `.vercel/output` | Managed hosted dashboard |
| Cloudflare Workers | `bun run build` | `dist/` | Worker-hosted dashboard with Hyperdrive |

The canonical portable path is the Docker image:

```bash
docker build -f apps/app/Dockerfile -t strait-app .
docker run --rm \
  -e AUTH_DATABASE_URL=postgres://user:pass@host:5432/strait \
  -e BETTER_AUTH_URL=http://localhost:3000 \
  -e BETTER_AUTH_SECRET="$(openssl rand -hex 32)" \
  strait-app .output/migrate.mjs
docker run --rm -p 3000:3000 \
  -e AUTH_DATABASE_URL=postgres://user:pass@host:5432/strait \
  -e BETTER_AUTH_URL=http://localhost:3000 \
  -e BETTER_AUTH_SECRET="$(openssl rand -hex 32)" \
  -e STRAIT_API_URL=http://host.docker.internal:8080 \
  strait-app
```

For the full self-hosted stack, see [SELFHOST.md](../../SELFHOST.md).

### Deploy with Docker

Use the Dockerfile for Render, Railway, Fly.io, Cloud Run, a VPS, k8s, or any
host that supports container deployment. The image builds the portable Node
server internally:

| Setting | Value |
|---|---|
| Dockerfile | `apps/app/Dockerfile` |
| Server entrypoint | `.output/server/index.mjs` |
| Container command | `.output/server/index.mjs` |

Use `bun run build:node` and `bun start` on direct Node hosts that run the app
without Docker.

### Deploy to Vercel

[![Deploy with Vercel](https://vercel.com/button)](https://vercel.com/new/clone?repository-url=https%3A%2F%2Fgithub.com%2Fstrait-dev%2Fstrait&project-name=strait-app&repository-name=strait-app&root-directory=apps%2Fapp&install-command=cd+..%2F..+%26%26+bun+install+--frozen-lockfile&build-command=cd+..%2F..+%26%26+cd+apps%2Fapp+%26%26+bun+run+build%3Avercel&env=AUTH_DATABASE_URL%2CBETTER_AUTH_URL%2CBETTER_AUTH_SECRET%2CSTRAIT_API_URL%2COIDC_ISSUER%2COIDC_AUDIENCE%2COIDC_PRIVATE_KEY_PEM)

Use Vercel as the managed convenience path. Keep these project settings:

| Setting | Value |
|---|---|
| Root directory | `apps/app` |
| Install command | `cd ../.. && bun install --frozen-lockfile` |
| Build command | `cd ../.. && cd apps/app && bun run build:vercel` |
| Output | Nitro/Vercel Build Output API (`.vercel/output`) |

Required environment variables:

| Variable | Notes |
|---|---|
| `AUTH_DATABASE_URL` | PostgreSQL connection string for Better Auth. |
| `BETTER_AUTH_URL` | Public dashboard origin, for example `https://dashboard.example.com`. |
| `BETTER_AUTH_SECRET` | 32+ character random string. Generate with `openssl rand -hex 32`. |
| `STRAIT_API_URL` | Public Strait API base URL. A Vercel-hosted dashboard cannot call `localhost`. |
| `OIDC_ISSUER` / `OIDC_AUDIENCE` / `OIDC_PRIVATE_KEY_PEM` | Required for dashboard-issued OIDC tokens. Must match the Go backend public key config. |

Optional variables enable OAuth, billing, email, Sentry, and PostHog. OAuth callback URLs must use the deployed dashboard origin from `BETTER_AUTH_URL`.

### Deploy to Cloudflare Workers

Use the Cloudflare target for Workers Builds or direct Wrangler deploys. This
target uses TanStack Start's Cloudflare Worker entrypoint and the Cloudflare Vite
plugin, so it produces `dist/` instead of Nitro Node output.

| Setting | Value |
|---|---|
| Root directory | `apps/app` |
| Install command | `bun install --frozen-lockfile` |
| Build command | `bun run build` |
| Preview deploy command | `bunx wrangler versions upload` |
| Production deploy command | `bunx wrangler deploy` |
| Worker config | `apps/app/wrangler.jsonc` |
| Worker entrypoint | `@tanstack/react-start/server-entry` |

Cloudflare Workers Builds can use the default `bun run build` command. That
command is intentionally Cloudflare-safe because Workers Builds commonly runs it
before `wrangler versions upload`.

Required Cloudflare secrets are set in the Workers dashboard under Variables and
Secrets. Non-secret defaults live in `wrangler.jsonc` so Cloudflare's deploy flow
can prompt users to override them.

## Tech Stack

| Technology | Purpose |
|---|---|
| TanStack Start | React 19 + Vite SSR framework |
| TanStack Router | File-based routing with loaders |
| TanStack Query | Server state management |
| Better Auth | Authentication (PostgreSQL + organization plugin) |
| Stripe | Billing, subscriptions, and usage-based metering |
| Effect | Error handling |
| Zustand | Client-side UI state |
| Recharts | Dashboard and billing charts |
| Biome | Linting and formatting |

## Project Structure

```text
src/
  routes/            File-based routing (TanStack Router)
    (auth)/           Unauthenticated routes (login, signup, 2FA, etc.)
    app/              Authenticated app shell
      billing/        Billing overview
      dlq/            Failed run review
      events/         Event stream
      jobs/            Job list + detail
      logs/            Log viewer
      runs/            Run list + detail
      schedules/       Schedule list + detail
      settings/        User/account settings
      webhooks/        Webhook management
      workflows/       Workflow list + detail
    onboarding/       Onboarding flow
  components/         UI components organized by domain
    (auth)/            Auth form components
    (settings)/        Settings page components
    billing/           Billing dashboard, budgets, alerts, charts
    common/            Sidebar, error states, skeletons, theme toggle
    dashboard/         Dashboard charts, detail sheets, status badge
    feature-gates/     Plan limit enforcement
    organization/      Org creation, switching
    project/           Project creation, settings, switcher
    tables/            Column definitions for data tables
    upgrade/           Plan selection, trial modal
  hooks/              Server functions + TanStack Query hooks
    api/               Entity hooks (jobs, runs, events, webhooks, etc.)
    auth/              Auth hooks (account, org, members, permissions)
    billing/           Billing hooks (usage, budget, limits, forecasts)
    subscription/      Subscription state management
  lib/                Shared utilities
    api-client.server  Server-only HTTP client for Go API
    effect-api.server  Effect-based API layer with Sentry reporting
    auth.server        Better Auth server instance
    status             Centralized status constants for all entities
    sentry             Sentry initialization and error capture
    kv.server          Upstash Redis KV client
    format             Number and date formatting helpers
  middlewares/        Server function middleware
    auth               Session validation, attaches user context
    require-access     IDOR protection (org/project ownership checks)
  stores/             Zustand client-side stores
  utils/              Constants and helpers
```

## Commands

| Command | Description |
|---|---|
| `bun dev` | Start dev server on port 5173 |
| `bun run build` | Production build for Cloudflare Workers |
| `bun run build:node` | Build Nitro `node-server` output |
| `bun run build:vercel` | Build Vercel output |
| `bun run build:cloudflare` | Build Cloudflare Worker output |
| `bun start` | Start the built Node server |
| `bun test` | Run Vitest tests |
| `bun run test:watch` | Run tests in watch mode |
| `bun run typecheck` | TypeScript check (tsgo) |
| `bun run biome:lint` | Lint with Biome |
| `bun run e2e:core:local` | Run backend-backed dashboard E2E tests with a managed local backend |
| `bun run run-all` | Biome fix + format + lint + typecheck |
| `bun run knip` | Detect unused code and dependencies |

## Environment Variables

For local development, copy `.env.example` to `apps/app/.env` and keep real
secrets out of git.

| Variable | Purpose |
|---|---|
| `AUTH_DATABASE_URL` | PostgreSQL connection string for the auth database |
| `BETTER_AUTH_URL` | Better Auth base URL |
| `BETTER_AUTH_SECRET` | Better Auth signing secret |
| `STRAIT_API_URL` | Go API backend URL (default: `http://localhost:8080`) |
| `INTERNAL_SECRET` | Shared secret for app-to-backend requests |
| `VITE_BASE_URL` | Frontend base URL |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | Google OAuth |
| `VITE_GOOGLE_CLIENT_ID` | Google One Tap (client-side) |
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` | GitHub OAuth |
| `STRIPE_SECRET_KEY` | Stripe API secret key |
| `STRIPE_PUBLISHABLE_KEY` | Stripe publishable key (client-side) |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook signature verification |
| `RESEND_API_KEY` | Transactional email via Resend |
| `VITE_SENTRY_DSN` | Sentry client-side DSN |
| `VITE_POSTHOG_KEY` / `VITE_POSTHOG_HOST` | PostHog analytics |

## Architecture

### Dual Database

The app connects to two separate PostgreSQL databases:

- **Auth DB** (`AUTH_DATABASE_URL`): managed by Better Auth, stores users, sessions, organizations, members, projects
- **Go Service DB**: managed by the Go backend (`apps/strait/`), stores jobs, runs, events, workflows

The app never writes to the Go service DB directly. All mutations go through server functions that call the Go API via `STRAIT_API_URL`.

### Authentication

Better Auth handles authentication with support for multiple providers, passkeys, and 2FA.

The `authMiddleware` validates sessions and attaches `user`, `session`, and `activeOrganizationId` to server function context. IDOR protection is enforced via `requireOrgAccess` and `requireProjectAccess` middleware.

### Data Fetching

Server functions use Effect for typed error handling, consumed via TanStack Query. See `src/hooks/` for examples.

### Billing

Billing is handled through Stripe. The dashboard includes usage tracking, spending limits, and plan management.

### Workspace Dependencies

| Package | Path | Purpose |
|---|---|---|
| `@strait/ui` | npm package | Shared component library (Tailwind v4, Base UI) |
| `@strait/transactional` | `packages/transactional` | Email templates (React Email) |

## Validation

Run focused checks before opening a frontend PR:

```bash
cd apps/app
bun run biome:lint
bun run typecheck
bun test
```

Run `bun run e2e:core:local` when touching flows that depend on the Go API.
