# @strait/billing

Shared billing constants and plan definitions for the Strait product. Contains the single source of truth for plan tiers (Free, Starter, Pro, Scale, Enterprise), pricing, feature limits, and plan metadata.

## Key exports

Single entry point:

```ts
import { PLANS, PLAN_KEYS, formatPlanPrice } from "@strait/billing/products";
```

### Exports from `./products`

- `PlanKey` -- union type: `"free" | "starter" | "pro" | "scale" | "enterprise"`
- `Plan` -- type describing a plan (name, prices, limits, features)
- `PLANS` -- record mapping each `PlanKey` to its `Plan` definition
- `PLAN_KEYS` -- ordered array of plan keys
- `formatPlanPrice()` -- formats a plan's price for display

Plan definitions include monthly/yearly pricing (in cents), trial eligibility, feature lists, and detailed limits (organizations, members, runs, retention, concurrency, etc.).

## Used by

- The marketing site (<https://github.com/strait-dev/website>) -- pricing page, pricing comparison tables, structured data
