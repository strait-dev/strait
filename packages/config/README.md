# @strait/config

Shared TypeScript configuration presets for the Strait monorepo. Provides base `tsconfig` settings that other packages extend.

## Source Of Truth

These files are the source of truth for TypeScript compiler defaults in workspace packages. Runtime application configuration lives elsewhere:

- Go service env vars: `apps/strait/internal/config/config.go`
- Dashboard env vars: `apps/app/README.md`

## Files

- `base.json` -- Base TypeScript config. Targets ES2017, enables strict mode, bundler module resolution, `noUncheckedIndexedAccess`, `verbatimModuleSyntax`, and other strict checks.
- `react-library.json` -- Extends `base.json`, adds `jsx: "react-jsx"`. Intended for React library packages.

## Used by

- `@strait/ui` -- npm-hosted design system package
- `packages/transactional` -- extends `base.json`

These are `tsconfig` presets only. No runtime code is exported.

## Commands

```bash
bun run --cwd packages/config clean
```
