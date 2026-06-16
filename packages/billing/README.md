# @strait/billing

Shared billing constants and generated plan definitions for Strait. The canonical source is `catalog/strait-pricing.json`; generated TypeScript and Go artifacts are checked in so app and backend code use the same launch catalog.

## Source Of Truth

Edit `catalog/strait-pricing.json` first. Do not edit generated catalog artifacts by hand.

## Commands

Generate and check artifacts:

```bash
bun run --cwd packages/billing generate
bun run --cwd packages/billing check:generated
```

Single entry point:

```ts
import { PLANS, PLAN_KEYS, formatPlanPrice } from "@strait/billing/products";
```

### Exports from `./products`

- `PlanKey` -- union type: `"free" | "starter" | "pro" | "scale" | "business" | "enterprise"`
- `Plan` -- type describing a plan (name, prices, limits, features)
- `PLANS` -- record mapping each `PlanKey` to its `Plan` definition
- `PLAN_KEYS` -- ordered array of plan keys
- `PLAN_API_RESPONSE` -- generated plan payload used by billing UI
- `ACTIVE_ADDONS` and `ROADMAP_ADDONS` -- add-on display metadata split by launch status
- `formatPlanPrice()` -- formats a plan's price for display

Plan definitions include monthly and annual pricing in cents, feature lists, roadmap display metadata, and detailed limits for orchestration runs, members, retention, concurrency, schedules, webhooks, worker connections, and API rate limits.

## Used by

- `apps/app` - billing UI, plan gates, usage screens
- `apps/strait/internal/billing` - cloud-edition billing enforcement through generated Go data
- `apps/docs/scripts/lint-docs.mjs` - pricing table drift checks
- The marketing site (<https://github.com/strait-dev/website>) -- pricing page, pricing comparison tables, structured data

## Validation

Run this after changing pricing, limits, add-ons, or plan names:

```bash
bun run --cwd packages/billing check:generated
bun run --cwd apps/docs lint
```
