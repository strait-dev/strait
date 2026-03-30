# Strait Website (`apps/website`)

Marketing website for Strait built with Astro and deployed to Cloudflare Workers.

## Stack

- **Framework:** Astro + React 19
- **Styling:** Tailwind CSS v4 + shared `@strait/ui`
- **Hosting:** Cloudflare Workers
- **Package manager:** Bun
- **Content model:**
  - **Landing/pricing/legal:** hardcoded in app code
  - **Blog:** Basehub-backed
- **Shared pricing source:** `@strait/billing/products`

## Important Project Notes

- Website defaults to **dark mode**.
- Website accent theme is controlled by `data-accent` in `BaseLayout.astro` (currently defaults to `teal`).
- CTA links to dashboard should use `dashboardHref(...)` from `src/lib/urls.ts`.
- Keep Basehub usage limited to blog surfaces (`/blog` and related components/routes).

## Directory Highlights

```txt
apps/website/
├── src/pages/                # Landing, pricing, legal, blog pages
├── src/layouts/              # BaseLayout, LandingLayout
├── src/components/           # Shared website components
├── src/lib/                  # URLs, metadata, structured-data helpers
└── public/                   # Static assets
```

## Environment Variables

| Variable | Purpose |
| --- | --- |
| `PUBLIC_WEBSITE_URL` | Canonical site URL for metadata/OG |
| `PUBLIC_DASHBOARD_URL` | Base URL for dashboard auth/CTA links |
| `PUBLIC_WEBSITE_ACCENT` | Optional accent override (`teal`, `blue`, `violet`, `emerald`) |
| `PUBLIC_GTM_ID` | Google Tag Manager integration |
| `BASEHUB_TOKEN` | Required only for blog CMS queries/build surfaces using Basehub |

## Commands

Run from repo root unless noted.

```bash
bun install
```

Inside `apps/website`:

| Command | Description |
| --- | --- |
| `bun run dev` | Start dev server |
| `bun run build` | Production build (`astro build`) |
| `bun run preview` | Preview production build |
| `bun run deploy` | Build and deploy to Cloudflare Workers |
| `bun run biome:lint` | Biome lint |
| `bun run typecheck` | Astro check |
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
