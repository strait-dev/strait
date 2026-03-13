# Strait Website (`apps/website`)

Marketing website for Strait built with Next.js App Router.

## Stack

- **Framework:** Next.js 16 + React 19
- **Styling:** Tailwind CSS v4 + shared `@strait/ui`
- **Package manager:** Bun
- **Content model:**
  - **Landing/pricing/legal:** hardcoded in app code
  - **Blog:** Basehub-backed
- **Shared pricing source:** `@strait/billing/products`

## Important Project Notes

- Website defaults to **dark mode**.
- Website accent theme is controlled by `data-accent` in `src/app/layout.tsx` (currently defaults to `teal`).
- CTA links to dashboard should use `dashboardHref(...)` from `src/lib/urls.ts`.
- Keep Basehub usage limited to blog surfaces (`/blog` and related components/routes).

## Directory Highlights

```txt
apps/website/
├── src/app/(landing)/        # Landing, pricing, legal, blog pages
├── src/app/layout.tsx        # Metadata, theme, GTM, global providers
├── src/components/           # Shared website components
├── src/config/site.ts        # Canonical site metadata config
├── src/lib/                  # URLs, metadata, structured-data helpers
└── public/                   # Static assets
```

## Environment Variables

| Variable | Purpose |
| --- | --- |
| `NEXT_PUBLIC_WEBSITE_URL` | Canonical site URL for metadata/OG |
| `NEXT_PUBLIC_DASHBOARD_URL` | Base URL for dashboard auth/CTA links |
| `NEXT_PUBLIC_WEBSITE_ACCENT` | Optional accent override (`teal`, `blue`, `violet`, `emerald`) |
| `BASEHUB_TOKEN` | Required only for blog CMS queries/build surfaces using Basehub |
| `NEXT_PUBLIC_GOOGLE_TAG_MANAGER_ID` | GTM integration |

## Commands

Run from repo root unless noted.

```bash
bun install
```

Inside `apps/website`:

| Command | Description |
| --- | --- |
| `bun run dev` | Start dev server |
| `bun run build` | Production build (`next build`) |
| `bun run start` | Start production server |
| `bun run lint` | Biome lint |
| `bun run typecheck` | TypeScript check |
| `bun run run-all` | Full local quality pass (fix/format/lint/typecheck) |

## QA Checklist (before PR)

```bash
cd apps/website
bun run run-all
bun run build
```

Also validate:
- Landing + pricing in dark mode
- Mobile header/nav interactions
- Blog routes still render when Basehub is configured
