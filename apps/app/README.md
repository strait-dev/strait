# Strait App

Job orchestration management UI built with TanStack Start. Create and monitor jobs, track runs in real time, manage workflows and schedules, configure webhooks, and view logs.

## Quick Start (local development)

```bash
# Install dependencies (from repo root)
bun install

# Start development server
cd apps/app
bun dev
```

The app runs at `http://localhost:5173`.

## Deploy to Cloudflare

[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=https://github.com/strait-dev/strait)

Clicking the button forks `strait-dev/strait` to your GitHub account and takes you through a Cloudflare Workers import. Because this repo is a Bun monorepo (Cloudflare does not yet support Bun workspace resolution in one-click deploys), the flow needs **one manual setting** before the first build:

1. During the Workers Builds import screen, set:
   - **Root directory**: `apps/app`
   - **Build command**: `cd ../.. && bun install --frozen-lockfile && cd apps/app && bun run build`
   - **Deploy command**: `npx wrangler deploy` *(default, leave as-is)*
2. When Cloudflare detects the `HYPERDRIVE` binding in `apps/app/wrangler.jsonc`, it will prompt you to create a new Hyperdrive config pointing at your Postgres (Neon, Supabase, Fly PG, or any Postgres with a connection string).
3. Cloudflare will prompt you for the non-secret variables declared in `apps/app/wrangler.jsonc` (`BETTER_AUTH_URL`, `STRAIT_API_URL`, OAuth client IDs, Stripe price IDs, etc.). Fill in the required ones; leave optional fields blank.
4. Confirm and deploy.

**After the first deploy**, open the Cloudflare dashboard → Workers → `strait-app` → Settings → Variables and Secrets, and add the following secrets:

| Secret | Required? | Notes |
|---|---|---|
| `BETTER_AUTH_SECRET` | yes | 32+ character random string. Generate with `openssl rand -hex 32`. |
| `OIDC_PRIVATE_KEY_PEM` | yes | PKCS#8 RSA private key used to sign MCP tokens. Must match the Go backend's public key. |
| `GOOGLE_CLIENT_SECRET` | only if Google OAuth is enabled | Paired with `GOOGLE_CLIENT_ID` above. |
| `GITHUB_CLIENT_SECRET` | only if GitHub OAuth is enabled | Paired with `GITHUB_CLIENT_ID` above. |
| `STRIPE_SECRET_KEY` | only if billing is enabled | Live or test Stripe secret key. |
| `STRIPE_WEBHOOK_SECRET` | only if billing is enabled | Stripe webhook signing secret. |
| `RESEND_API_KEY` | only if transactional email is enabled | Resend API key for magic links, invites, password resets. |
| `VITE_SENTRY_DSN` | optional | Client-side Sentry DSN. |
| `VITE_POSTHOG_KEY` | optional | Client-side PostHog key. |

Then redeploy once so the Worker picks up the new secrets: either push any commit, or hit **Deployments → Retry** in the Cloudflare dashboard.

**Your Strait API must be reachable from the Worker.** If you are running the Strait API locally via `docker compose -f docker-compose.selfhost.yml up`, expose it with a tunnel (for example `cloudflared tunnel`) so the deployed Worker can call it, and set `STRAIT_API_URL` accordingly. If you are running the API on a public host, just point `STRAIT_API_URL` at it.

For the fully containerized alternative (running the dashboard itself in Docker alongside the API), see [SELFHOST.md](../../SELFHOST.md).

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
| `bun build` | Production build (Cloudflare Workers) |
| `bun start` | Start production server |
| `bun test` | Run Vitest tests |
| `bun run test:watch` | Run tests in watch mode |
| `bun run typecheck` | TypeScript check (tsgo) |
| `bun run biome:lint` | Lint with Biome |
| `bun run run-all` | Biome fix + format + lint + typecheck |
| `bun run knip` | Detect unused code and dependencies |

## Environment Variables

Secrets are managed via Infisical (project: `strait`, environments: `dev`/`stg`/`prd`).

For local development: `infisical run --env=dev -- bun dev`

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
| `STRIPE_*_PRICE_ID` | Stripe Price IDs for plan tiers and addons |
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
| `@strait/ui` | `packages/ui` | Shared component library (Tailwind v4, Radix) |
| `@strait/transactional` | `packages/transactional` | Email templates (React Email) |
