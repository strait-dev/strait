# Strait App

Job orchestration management UI built with TanStack Start.

## Quick Start

```bash
# Install dependencies (from repo root)
bun install

# Start local infrastructure and API first
docker compose -f ../strait/docker-compose.yml up -d
cd ../strait && INTERNAL_SECRET=strait-local-internal-secret-32chars JWT_SIGNING_KEY=strait-local-jwt-signing-key-32chars DATABASE_URL=postgres://strait:strait@localhost:5432/strait?sslmode=disable REDIS_URL=redis://localhost:6379 go run ./cmd/strait --mode all

# Start development server
cd apps/app
bun run dev
```

The app runs at `http://localhost:5173`.

## Tech Stack

| Technology | Purpose |
|---|---|
| TanStack Start | React 19 + Vite SSR framework |
| TanStack Router | File-based routing with loaders |
| TanStack Query | Server state (5-minute staleTime) |
| Better Auth | Authentication (PostgreSQL + organization plugin) |
| Stripe | Billing, subscriptions, and usage-based metering |
| Effect | Typed error modeling in server functions |
| Zustand | Client-side UI state |
| Recharts | Dashboard and billing charts |
| Biome | Linting and formatting |

## Project Structure

```
src/
  routes/            File-based routing (TanStack Router)
    (auth)/           Unauthenticated routes (login, signup, 2FA, etc.)
    app/              Authenticated app shell
      billing/        Billing overview
      dlq/            Dead-letter queue
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
| `bun run dev` | Bootstrap Better Auth locally, seed a local dev user, and start the app on port 5173 |
| `bun run dev:doppler` | Start the dev server with Doppler-injected envs (internal use) |
| `bun build` | Production build (Vercel preset) |
| `bun start` | Start production server |
| `bun test` | Run Vitest tests |
| `bun run test:watch` | Run tests in watch mode |
| `bun run typecheck` | TypeScript check (tsgo) |
| `bun run biome:lint` | Lint with Biome |
| `bun run run-all` | Biome fix + format + lint + typecheck |
| `bun run knip` | Detect unused code and dependencies |

## Environment Variables

`bun run dev` is local-first and does not require Doppler. It defaults to:

- `AUTH_DATABASE_URL=postgresql://strait:strait@localhost:5432/strait`
- `BETTER_AUTH_URL=http://localhost:5173`
- `BETTER_AUTH_SECRET=strait-local-better-auth-secret-32chars`
- `DISABLE_EMAIL_VERIFICATION=true`
- `DISABLE_POLAR_BILLING=true`
- `INTERNAL_SECRET=strait-local-internal-secret-32chars`
- `STRAIT_API_URL=http://127.0.0.1:8080`
- `LOCAL_DEV_USER_EMAIL=dev@local.strait`
- `LOCAL_DEV_USER_PASSWORD=devpassword123`
- `LOCAL_DEV_USER_NAME=Local Dev User`

Before Vite starts, `bun run dev` now:

- waits for local PostgreSQL
- runs Better Auth schema bootstrap/migrations
- seeds a deterministic local user, workspace, and default project
- syncs the default project to the Go API when `STRAIT_API_URL` is reachable

Set env vars explicitly if you need to override those defaults. `bun run dev:doppler` remains available for internal Doppler-backed development.

| Variable | Purpose |
|---|---|
| `AUTH_DATABASE_URL` | PostgreSQL connection string for the auth database |
| `BETTER_AUTH_URL` | Better Auth base URL |
| `BETTER_AUTH_SECRET` | Better Auth signing secret |
| `DISABLE_EMAIL_VERIFICATION` | Set to `true` in local/self-hosted development to allow email/password sign-up without verification emails |
| `DISABLE_POLAR_BILLING` | Set to `true` in local/self-hosted development to disable Polar billing bootstrap and billing portal calls |
| `LOCAL_DEV_USER_EMAIL` | Default local seeded user email for `bun run dev` |
| `LOCAL_DEV_USER_PASSWORD` | Default local seeded user password for `bun run dev` |
| `LOCAL_DEV_USER_NAME` | Default local seeded user display name for `bun run dev` |
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

- **Auth DB** (`AUTH_DATABASE_URL`) — for local development this should point to the Docker PostgreSQL instance at `localhost:5432`; Better Auth stores users, sessions, organizations, members, projects there
- **Go Service DB** — managed by the Go backend (`apps/strait/`), stores jobs, runs, events, workflows

The app never writes to the Go service DB directly. All mutations go through server functions that call the Go API via `STRAIT_API_URL`.

### Authentication

Better Auth with plugins: email/password, magic link, passkey (WebAuthn), Google OAuth, GitHub OAuth, Google One Tap, 2FA, and organizations. Stripe billing is handled via standalone server functions (Checkout Sessions, Customer Portal) and a Go backend webhook handler.

The `authMiddleware` validates sessions and attaches `user`, `session`, and `activeOrganizationId` to server function context. IDOR protection is enforced via `requireOrgAccess` and `requireProjectAccess` middleware.

### Data Fetching

Server functions wrapped in Effect for typed error handling, consumed via TanStack Query:

```typescript
// Server function with IDOR protection
const getProjectBudgetServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    await requireProjectAccess(context.user.id, data.projectId, activeOrgId);
    return await runWithFallback(
      apiEffect<ProjectBudgetData>("/v1/project-budget", {
        params: { project_id: data.projectId },
      }),
      null
    );
  });

// Query options consumed by components
export const projectBudgetQueryOptions = (projectId: string) =>
  queryOptions({
    queryKey: queryKeys.billing.projectBudget(projectId).queryKey,
    queryFn: () => getProjectBudgetServerFn({ data: { projectId } }),
  });
```

### Billing

Five plan tiers: `free < starter < pro < scale < enterprise`. Billing is managed through Stripe with webhook sync to the Go backend. Checkout uses Stripe-hosted pages (Checkout Sessions), subscription management uses the Stripe Customer Portal, and compute usage is tracked via Stripe Billing Meters. The billing dashboard includes usage charts, spending limits, anomaly detection, and project budgets.

### Code Conventions

- **Components**: `const` arrow functions with `export default` at end of file
- **Routes**: `function` keyword for route components
- **Hooks**: `export const` for all hook exports
- **Search schemas**: exported from route files as `export const searchSchema`
- **Status constants**: centralized in `lib/status.ts`

### Workspace Dependencies

| Package | Path | Purpose |
|---|---|---|
| `@strait/ui` | `packages/ui` | Shared component library (Tailwind v4, Radix) |
| `@strait/transactional` | `packages/transactional` | Email templates (React Email) |
| `@strait/utils` | `packages/utils` | Shared utilities |
