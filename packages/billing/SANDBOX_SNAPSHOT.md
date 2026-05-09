# Stripe Sandbox Snapshot

Canonical Stripe sandbox state after Wave 2 Phase 3. The resolver fetches live
Stripe Price IDs from these lookup keys; this file is for human reference and
audit, not for code import.

## Account

- Mode: test
- Meter: `mtr_test_61Ue9XDEGj7VaibVk41CY4bMQR1xe9dI` (run usage)

## Tier prices

| Tier       | Monthly lookup key        | Monthly price ID                 | Monthly amount | Annual lookup key        | Annual price ID                  | Annual amount |
|------------|---------------------------|----------------------------------|----------------|--------------------------|----------------------------------|---------------|
| Free       | `strait_free_monthly`     | `price_1TUlbCCY4bMQR1xeZJ163S5T` | $0             | —                        | —                                | —             |
| Starter    | `strait_starter_monthly`  | `price_1TUlbDCY4bMQR1xeR9ukiIZ4` | $19            | `strait_starter_annual`  | `price_1TV8K3CY4bMQR1xeGPvq8SRZ` | $180          |
| Pro        | `strait_pro_monthly`      | `price_1TUlbFCY4bMQR1xeiy4iXhOT` | $99            | `strait_pro_annual`      | `price_1TV8K8CY4bMQR1xeHhc79Zog` | $948          |
| Scale      | `strait_scale_monthly`    | `price_1TUlbHCY4bMQR1xe3pY8Jf1l` | $299           | `strait_scale_annual`    | `price_1TV8KDCY4bMQR1xe7UshXPj2` | $2,868        |
| Business   | `strait_business_monthly` | `price_1TUlbKCY4bMQR1xeozU9kimD` | $499           | `strait_business_annual` | `price_1TV8KHCY4bMQR1xe8TNDaJER` | $4,788        |
| Enterprise | —                         | —                                | quoted         | —                        | —                                | quoted        |

## Graduated overage prices

All overage prices use `tiers_mode=graduated`, `billing_scheme=tiered`,
`usage_type=metered`, with the meter above. Tier 1 cliff is the plan's included
allowance at $0; tier 2 charges the per-run rate (in cents) for any usage
beyond the cliff.

| Tier       | Lookup key                | Price ID                         | Cliff (runs / month) | Per-run rate    |
|------------|---------------------------|----------------------------------|----------------------|-----------------|
| Free       | `strait_overage_free`     | `price_1TV8JxCY4bMQR1xeVEduCdbP` | 5,000                | 0.05¢ ($0.50/1K) |
| Starter    | `strait_overage_starter`  | `price_1TV8JSCY4bMQR1xepLZCDjCm` | 50,000               | 0.04¢ ($0.40/1K) |
| Pro        | `strait_overage_pro`      | `price_1TV8JhCY4bMQR1xeVmJurYiw` | 1,000,000            | 0.02¢ ($0.20/1K) |
| Scale      | `strait_overage_scale`    | `price_1TV8JlCY4bMQR1xe22fL5wlq` | 5,000,000            | 0.006¢ ($0.06/1K) |
| Business   | `strait_overage_business` | `price_1TV8JrCY4bMQR1xeFJOZNSM9` | 25,000,000           | 0.003¢ ($0.03/1K) |

## Deactivated (superseded) prices

These were active in the sandbox before Wave 2 Phase 3 and are now `active:false`.
Listed here so an operator can confirm they were retired intentionally.

- Wrong-amount annuals (replaced by corrected $180/$948/$2,868/$4,788 values):
  - `price_1TUlbECY4bMQR1xeKucCs3XA` Starter $182.40
  - `price_1TUlbGCY4bMQR1xexsnhEN1W` Pro $950.40
  - `price_1TUlbICY4bMQR1xefzwP9Asw` Scale $2,870.40
  - `price_1TUlbLCY4bMQR1xeKStu2r2y` Business $4,790.40
- Non-graduated metered overage prices (replaced by graduated equivalents):
  - `price_1TUlbNCY4bMQR1xeNSH4K6Hf` Free product
  - `price_1TUlbOCY4bMQR1xeg3kX1tn8` Starter product
  - `price_1TUlbQCY4bMQR1xeIlNEI1QQ` Pro product
  - `price_1TUlbRCY4bMQR1xeVkkIJUiW` Scale product
  - `price_1TUlbTCY4bMQR1xeMyEQWkfN` Business product

## Reproducing this state

The sandbox state was rebuilt via the `mcp__stripe-sandbox__stripe_api_execute`
MCP server during Wave 2 Phase 3. To reproduce on a fresh Stripe sandbox account:

1. Create six tier products (Free, Starter, Pro, Scale, Business, Enterprise).
2. Create one monthly licensed price per non-Enterprise tier at the amounts
   in the tier table.
3. Create one annual licensed price for Starter, Pro, Scale, Business at the
   amounts in the tier table.
4. Create five graduated metered prices using the meter ID above, with the
   cliffs and per-run rates in the overage table. Tier 1 must use `flat_amount`
   or `unit_amount_decimal=0` up to the cliff; tier 2 charges
   `unit_amount_decimal` matching the per-run rate.
5. Assign canonical lookup keys (no `_v2` suffix) to each price using
   `transfer_lookup_key=true`.

## Live equivalence

The Stripe live account is reconciled to match this sandbox shape as of Wave 2
Phase 4. Live had zero active subscriptions at the time of the migration, so
the rollout was non-disruptive.

Live meter: `mtr_61UeEtSZ9XnfVRTnl41CtZGvNNzEBGka`.

Live tier prices (canonical lookup keys assigned, amounts match the table above):

| Tier     | Monthly price ID                  | Annual price ID                   |
|----------|-----------------------------------|-----------------------------------|
| Free     | `price_1TUlbUCtZGvNNzEBrBhekZCK`  | —                                 |
| Starter  | `price_1TUlbVCtZGvNNzEBobah7ej9`  | `price_1TUqwmCtZGvNNzEBepOg1VMR`  |
| Pro      | `price_1TUlbXCtZGvNNzEBKcX6kj7E`  | `price_1TUqwnCtZGvNNzEBKvQe2v7g`  |
| Scale    | `price_1TUlbaCtZGvNNzEBIdLJF65I`  | `price_1TUqwoCtZGvNNzEBP0SHFWNm`  |
| Business | `price_1TUlbcCtZGvNNzEBOyK10aBk`  | `price_1TUqwpCtZGvNNzEBBmsB7huQ`  |

Live graduated overage prices (created in Phase 4 with `transfer_lookup_key=true`):

| Tier     | Lookup key                | Price ID                          |
|----------|---------------------------|-----------------------------------|
| Free     | `strait_overage_free`     | `price_1TV8gXCtZGvNNzEBGO8X6D3C`  |
| Starter  | `strait_overage_starter`  | `price_1TV8geCtZGvNNzEBw7alswDT`  |
| Pro      | `strait_overage_pro`      | `price_1TV8gqCtZGvNNzEBMYpObkZ6`  |
| Scale    | `strait_overage_scale`    | `price_1TV8gvCtZGvNNzEBvQRRoeVu`  |
| Business | `strait_overage_business` | `price_1TV8h0CtZGvNNzEBlPPSJnZ4`  |

Deactivated in live during Phase 4:

- Wrong-amount annuals: `price_1TUlbWCtZGvNNzEBG91BtY5f` ($182.40 Starter),
  `price_1TUlbYCtZGvNNzEBFGqKqGWd` ($950.40 Pro),
  `price_1TUlbbCtZGvNNzEBZWIa9M3Q` ($2,870.40 Scale),
  `price_1TUlbdCtZGvNNzEBoMAgGZtz` ($4,790.40 Business).
- Per-unit overage prices superseded by graduated equivalents:
  `price_1TUqxDCtZGvNNzEBsBA3wlKm` (Free), `price_1TUqxECtZGvNNzEBgcNM8LYP`
  (Starter), `price_1TUqxFCtZGvNNzEBdbFlObSg` (Pro),
  `price_1TUqxFCtZGvNNzEBmtolHaBY` (Scale),
  `price_1TUqxGCtZGvNNzEBazs09bdf` (Business).
