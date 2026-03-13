# Strait Dashboard

> Job orchestration management UI for Strait

## Quick Start

```bash
# Install dependencies (from repo root)
bun install

# Start development server
cd apps/app
bun dev
```

The app runs at `http://localhost:3000`.

## Tech Stack

| Technology | Purpose |
|------------|---------|
| **TanStack Start** | React 19 + Vite SSR framework |
| **TanStack Router** | File-based routing with loaders |
| **TanStack Query** | Server state management |
| **Better Auth** | Authentication (PostgreSQL + Drizzle) |
| **Polar** | Billing and subscriptions |
| **Zustand** | UI state |

## Project Structure

```
src/
├── routes/         # File-based routing
├── components/     # Domain UI (settings, organization, auth, entity)
├── hooks/          # Server functions + TanStack Query hooks
├── stores/         # Zustand stores
├── lib/            # Auth, subscription, sentry, organization
├── middlewares/     # Auth middleware
├── actions/        # Server actions
└── utils/          # Constants, formatting
```

## Development

### Prerequisites

- Bun 1.2.x
- Environment variables (via Infisical CLI)

### Commands

| Command | Description |
|---------|-------------|
| `bun dev` | Start dev server |
| `bun build` | Production build |
| `bun test` | Run Vitest tests |
| `bun typecheck` | TypeScript check |
| `bun run run-all` | Biome lint/format + typecheck |

### Environment Variables

```bash
# App
VITE_BASE_URL=              # Auth redirects
AUTH_DATABASE_URL=           # PostgreSQL for Better Auth
STRAIT_API_URL=              # Strait Go API base URL
STRAIT_INTERNAL_SECRET=      # Internal API secret
# Sentry (optional locally)
VITE_SENTRY_DSN=
VITE_SENTRY_ENVIRONMENT=
VITE_SENTRY_ENABLED=false
```

## Architecture

### Authentication

Better Auth with organization and Polar plugins, backed by a separate PostgreSQL database managed in `packages/auth/`.

```typescript
// Server-side session check
const session = await getSession()

// Route guard
export const Route = createFileRoute('/app/settings')({
  beforeLoad: async ({ context }) => {
    if (!context.session) {
      throw redirect({ to: '/login' })
    }
  },
})
```

### Data Fetching

Server functions with TanStack Query for the Strait Go API:

```typescript
// In a route loader
export const Route = createFileRoute('/app/jobs')({
  loader: async ({ context }) => {
    return context.queryClient.ensureQueryData(jobsQueryOptions())
  },
})

// In a component
const { data } = useSuspenseQuery(jobsQueryOptions())
```

### Feature Gating

Gate features based on subscription plan:

```typescript
const { hasAccess } = useFeatureAccess('advanced_reports')

<InlineFeatureGate feature="advanced_reports">
  <AdvancedFeature />
</InlineFeatureGate>
```

## Resources

- [App Guidelines](.cursor/rules/app-guidelines.mdc)
- [Route Best Practices](.cursor/rules/app-route-best-practices.mdc)
- [Claude Skills](.claude/skills/app-guidelines.md)
- [Project Guide](CLAUDE.md)
