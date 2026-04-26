# @strait/config

Shared TypeScript configuration presets for the Strait monorepo. Provides base `tsconfig` settings that other packages extend.

## Files

- `base.json` -- Base TypeScript config. Targets ES2017, enables strict mode, bundler module resolution, `noUncheckedIndexedAccess`, `verbatimModuleSyntax`, and other strict checks.
- `react-library.json` -- Extends `base.json`, adds `jsx: "react-jsx"`. Intended for React library packages.

## Used by

- `packages/ui` -- extends `react-library.json`
- `packages/transactional` -- extends `base.json`

These are `tsconfig` presets only. No runtime code is exported.
