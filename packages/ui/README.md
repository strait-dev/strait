# @strait/ui

Shared React component library for the Strait monorepo. Built on Tailwind CSS v4, Radix primitives, React Aria, and shadcn/ui patterns. Provides 90+ components covering forms, data display, navigation, overlays, and layout.

## Key exports

Components are exported individually via subpath imports:

```ts
import { Button } from "@strait/ui/components/button";
import { Dialog } from "@strait/ui/components/dialog";
import { DataTable } from "@strait/ui/components/data-table";
```

Additional exports:

- `@strait/ui/utils` -- `cn()` and shared utilities
- `@strait/ui/css` / `@strait/ui/globals.css` -- global stylesheet
- `@strait/ui/tailwind.config` -- shared Tailwind v4 config
- `@strait/ui/postcss` -- PostCSS config
- `@strait/ui/toast` -- toast system

## Notable dependencies

- `@tanstack/react-table` (data tables)
- `react-day-picker`, `react-aria-components` (date pickers, calendars)
- `recharts` (charts)
- `motion` (animations)
- `vaul` (drawers)
- `cmdk` (command palette)
- `sonner` (toasts)
- `react-hook-form` (form state)

## Used by

- `apps/app` -- primary consumer (dashboard, settings, auth flows)
- `apps/website` -- marketing site

## Development

```sh
bun run typecheck     # type-check with tsgo
bun run biome:lint    # lint
bun run biome:format  # format
```
