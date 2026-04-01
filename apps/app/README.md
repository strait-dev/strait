# Strait App

Job orchestration management UI built with TanStack Start.

## Quick Start

```bash
# Install dependencies (from repo root)
bun install

# Start development server
cd apps/app
bun dev
```

The app runs at `http://localhost:5173`.

## Tech Stack

| Technology | Purpose |
|---|---|
| TanStack Start | React 19 + Vite SSR framework |
| TanStack Router | File-based routing with loaders |
| TanStack Query | Server state (5-minute staleTime) |
| Better Auth | Authentication (PostgreSQL + organization plugin) |
| Polar | Billing and subscriptions |
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

Secrets are managed via Doppler (project: `strait`, configs: `dev`/`stg`/`prd`).

For local development: `doppler run -- bun dev`

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
| `POLAR_ACCESS_TOKEN` | Polar billing API token |
| `POLAR_SERVER` | `sandbox` or `production` |
| `POLAR_WEBHOOK_SECRET` | Polar webhook validation |
| `RESEND_API_KEY` | Transactional email via Resend |
| `VITE_SENTRY_DSN` | Sentry client-side DSN |
| `VITE_POSTHOG_KEY` / `VITE_POSTHOG_HOST` | PostHog analytics |

## Architecture

### Dual Database

The app connects to two separate PostgreSQL databases:

- **Auth DB** (`AUTH_DATABASE_URL`) — managed by Better Auth, stores users, sessions, organizations, members, projects
- **Go Service DB** — managed by the Go backend (`apps/strait/`), stores jobs, runs, events, workflows

The app never writes to the Go service DB directly. All mutations go through server functions that call the Go API via `STRAIT_API_URL`.

### Authentication

Better Auth with plugins: email/password, magic link, passkey (WebAuthn), Google OAuth, GitHub OAuth, Google One Tap, 2FA, organizations, and Polar billing.

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

Four plan tiers: `free < starter < pro < enterprise`. Billing is managed through Polar with webhook sync. The billing dashboard includes usage charts, spending limits, anomaly detection, project budgets, and referral tracking.

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
