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
- `PlanApiResponse` -- type of each `PLAN_API_RESPONSE` entry
- `PlanLookupKeys` -- type describing a plan's Stripe price lookup keys (`monthly`, `annual`, `overage`)
- `PLAN_LOOKUP_KEYS` -- record mapping each `PlanKey` to its Stripe monthly/annual/overage price lookup keys. This is what billing code uses to resolve the correct Stripe Price via lookup key instead of a hardcoded price ID -- treat changes here as operationally significant.
- `ACTIVE_ADDONS` and `ROADMAP_ADDONS` -- add-on display metadata split by launch status
- `AddonKey`, `ActiveAddonKey`, `RoadmapAddonKey` -- add-on key union types (all add-ons, launched only, roadmap only)
- `ADDON_KEYS`, `ACTIVE_ADDON_KEYS`, `ROADMAP_ADDON_KEYS` -- ordered arrays of the above key types
- `BillingAddon`, `ActiveBillingAddon`, `RoadmapBillingAddon` -- types describing an add-on's display name, Stripe lookup key, pack size, price, and availability
- `ROADMAP_FEATURES` -- flat list of feature names slated for future plans (SSO/SAML, SCIM, IP allowlisting, and similar)
- `PRICING_CATALOG_VERSION` -- catalog version string from `catalog/strait-pricing.json`
- `PRICING_CATALOG_HASH` -- SHA-256 of the source catalog, used to detect drift between the JSON source and generated artifacts
- `METERED_UNIT` -- the billing unit name (`"orchestration_run"`)
- `formatPlanPrice()` -- formats a plan's price for display given a billing interval
- `formatPrice()` / `formatPriceWithCents()` -- lower-level currency formatting helpers used by `formatPlanPrice()` and available for direct use

Plan definitions include monthly and annual pricing in cents, feature lists, roadmap display metadata, and detailed limits for orchestration runs, members, retention, concurrency, schedules, webhooks, worker connections, and API rate limits.

Note: this package's `build` and `typecheck` scripts are no-ops (there is no build step or type-check step beyond `check:generated`).

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
