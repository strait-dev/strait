import type Stripe from "stripe";
import { describe, expect, it } from "vitest";
import {
  DEFAULT_RANK,
  fromUnix,
  getFirstItem,
  SUBSCRIPTION_RANK,
  selectBestSubscription,
  toNormalizedSubscription,
} from "./subscription-helpers";

/**
 * Factory for creating minimal Stripe subscription objects for testing.
 * Only includes fields that the helpers actually read.
 */
const makeSub = (
  overrides: {
    id?: string;
    status?: Stripe.Subscription.Status;
    cancel_at_period_end?: boolean;
    trial_end?: number;
    items?: Array<{
      id?: string;
      current_period_end?: number;
      current_period_start?: number;
      price?: {
        id?: string;
        lookup_key?: string;
        recurring?: { interval?: string };
      };
    }>;
  } = {}
): Stripe.Subscription =>
  ({
    id: overrides.id ?? "sub_test",
    status: overrides.status ?? "active",
    cancel_at_period_end: overrides.cancel_at_period_end ?? false,
    trial_end: overrides.trial_end ?? 0,
    items: {
      data: (
        overrides.items ?? [
          {
            id: "si_test",
            current_period_end: 1_735_689_600, // 2025-01-01
            current_period_start: 1_733_011_200, // 2024-12-01
            price: {
              id: "price_starter_monthly",
              recurring: { interval: "month" },
            },
          },
        ]
      ).map((item) => ({
        id: item.id ?? "si_test",
        current_period_end: item.current_period_end ?? 0,
        current_period_start: item.current_period_start ?? 0,
        price: item.price ?? null,
      })),
    },
  }) as unknown as Stripe.Subscription;

// ---------------------------------------------------------------------------
// fromUnix
// ---------------------------------------------------------------------------
describe("fromUnix", () => {
  it("converts a Unix timestamp to a Date", () => {
    const date = fromUnix(1_735_689_600);
    expect(date).toBeInstanceOf(Date);
    expect(date?.toISOString()).toBe("2025-01-01T00:00:00.000Z");
  });

  it("returns null for zero", () => {
    expect(fromUnix(0)).toBeNull();
  });

  it("returns null for null", () => {
    expect(fromUnix(null)).toBeNull();
  });

  it("returns null for undefined", () => {
    expect(fromUnix(undefined)).toBeNull();
  });

  it("handles negative timestamps", () => {
    const date = fromUnix(-1);
    expect(date).toBeInstanceOf(Date);
    expect(date?.getTime()).toBe(-1000);
  });
});

// ---------------------------------------------------------------------------
// getFirstItem
// ---------------------------------------------------------------------------
describe("getFirstItem", () => {
  it("returns the first subscription item", () => {
    const sub = makeSub();
    const item = getFirstItem(sub);
    expect(item).not.toBeNull();
    expect(item?.id).toBe("si_test");
  });

  it("returns null when items.data is empty", () => {
    const sub = makeSub({ items: [] });
    expect(getFirstItem(sub)).toBeNull();
  });

  it("returns null when items is undefined", () => {
    const sub = { items: undefined } as unknown as Stripe.Subscription;
    expect(getFirstItem(sub)).toBeNull();
  });

  it("returns the first of multiple items", () => {
    const sub = makeSub({
      items: [
        { id: "si_first", price: { id: "price_a" } },
        { id: "si_second", price: { id: "price_b" } },
      ],
    });
    const item = getFirstItem(sub);
    expect(item?.id).toBe("si_first");
  });
});

// ---------------------------------------------------------------------------
// toNormalizedSubscription
// ---------------------------------------------------------------------------
describe("toNormalizedSubscription", () => {
  it("normalizes a complete subscription", () => {
    const sub = makeSub({
      id: "sub_123",
      status: "active",
      cancel_at_period_end: false,
      trial_end: 0,
      items: [
        {
          current_period_end: 1_735_689_600,
          price: {
            id: "price_pro_monthly",
            lookup_key: "strait_pro_monthly",
            recurring: { interval: "month" },
          },
        },
      ],
    });

    const result = toNormalizedSubscription(sub);

    expect(result).toEqual({
      id: "sub_123",
      status: "active",
      productId: "price_pro_monthly",
      priceId: "price_pro_monthly",
      lookupKey: "strait_pro_monthly",
      currentPeriodEnd: new Date("2025-01-01T00:00:00.000Z"),
      cancelAtPeriodEnd: false,
      recurringInterval: "month",
      trialEnd: null,
    });
  });

  it("handles yearly interval", () => {
    const sub = makeSub({
      items: [
        {
          current_period_end: 1_735_689_600,
          price: {
            id: "price_starter_yearly",
            recurring: { interval: "year" },
          },
        },
      ],
    });

    const result = toNormalizedSubscription(sub);
    expect(result.recurringInterval).toBe("year");
    expect(result.priceId).toBe("price_starter_yearly");
  });

  it("handles trialing subscription with trial end", () => {
    const trialEnd = 1_738_368_000; // 2025-02-01
    const sub = makeSub({
      status: "trialing",
      trial_end: trialEnd,
    });

    const result = toNormalizedSubscription(sub);
    expect(result.status).toBe("trialing");
    expect(result.trialEnd).toEqual(new Date("2025-02-01T00:00:00.000Z"));
  });

  it("handles canceled subscription with cancel_at_period_end", () => {
    const sub = makeSub({
      status: "active",
      cancel_at_period_end: true,
    });

    const result = toNormalizedSubscription(sub);
    expect(result.cancelAtPeriodEnd).toBe(true);
  });

  it("handles subscription with no items", () => {
    const sub = makeSub({ items: [] });
    const result = toNormalizedSubscription(sub);

    expect(result.priceId).toBe("");
    expect(result.productId).toBe("");
    expect(result.lookupKey).toBe("");
    expect(result.currentPeriodEnd).toBeNull();
    expect(result.recurringInterval).toBeNull();
  });

  it("handles subscription item with no price", () => {
    const sub = makeSub({
      items: [{ current_period_end: 1_735_689_600 }],
    });
    const result = toNormalizedSubscription(sub);

    expect(result.priceId).toBe("");
    expect(result.recurringInterval).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// SUBSCRIPTION_RANK
// ---------------------------------------------------------------------------
describe("SUBSCRIPTION_RANK", () => {
  it("ranks active statuses at 0", () => {
    expect(SUBSCRIPTION_RANK.active).toBe(0);
    expect(SUBSCRIPTION_RANK.trialing).toBe(0);
    expect(SUBSCRIPTION_RANK.past_due).toBe(0);
    expect(SUBSCRIPTION_RANK.incomplete).toBe(0);
    expect(SUBSCRIPTION_RANK.unpaid).toBe(0);
  });

  it("ranks canceled/paused at 1", () => {
    expect(SUBSCRIPTION_RANK.canceled).toBe(1);
    expect(SUBSCRIPTION_RANK.paused).toBe(1);
  });

  it("ranks incomplete_expired at 2", () => {
    expect(SUBSCRIPTION_RANK.incomplete_expired).toBe(2);
  });

  it("DEFAULT_RANK is 3 for unknown statuses", () => {
    expect(DEFAULT_RANK).toBe(3);
  });
});

// ---------------------------------------------------------------------------
// selectBestSubscription
// ---------------------------------------------------------------------------
describe("selectBestSubscription", () => {
  it("returns null for empty list", () => {
    expect(selectBestSubscription([])).toBeNull();
  });

  it("returns the only subscription for single-element list", () => {
    const sub = makeSub({ id: "sub_only" });
    const result = selectBestSubscription([sub]);
    expect(result?.id).toBe("sub_only");
  });

  it("prefers active over canceled", () => {
    const canceled = makeSub({
      id: "sub_canceled",
      status: "canceled",
      items: [{ current_period_end: 1_735_689_600 }],
    });
    const active = makeSub({
      id: "sub_active",
      status: "active",
      items: [{ current_period_end: 1_733_011_200 }],
    });

    // Active wins even with an earlier period end
    const result = selectBestSubscription([canceled, active]);
    expect(result?.id).toBe("sub_active");
  });

  it("prefers trialing over canceled", () => {
    const canceled = makeSub({ id: "sub_canceled", status: "canceled" });
    const trialing = makeSub({ id: "sub_trialing", status: "trialing" });

    const result = selectBestSubscription([canceled, trialing]);
    expect(result?.id).toBe("sub_trialing");
  });

  it("prefers past_due over canceled", () => {
    const canceled = makeSub({ id: "sub_canceled", status: "canceled" });
    const pastDue = makeSub({ id: "sub_past_due", status: "past_due" });

    const result = selectBestSubscription([canceled, pastDue]);
    expect(result?.id).toBe("sub_past_due");
  });

  it("prefers canceled over incomplete_expired", () => {
    const expired = makeSub({
      id: "sub_expired",
      status: "incomplete_expired",
    });
    const canceled = makeSub({ id: "sub_canceled", status: "canceled" });

    const result = selectBestSubscription([expired, canceled]);
    expect(result?.id).toBe("sub_canceled");
  });

  it("breaks ties by latest period end (within same rank)", () => {
    const older = makeSub({
      id: "sub_older",
      status: "active",
      items: [{ current_period_end: 1_733_011_200, price: { id: "price_a" } }],
    });
    const newer = makeSub({
      id: "sub_newer",
      status: "active",
      items: [{ current_period_end: 1_735_689_600, price: { id: "price_b" } }],
    });

    const result = selectBestSubscription([older, newer]);
    expect(result?.id).toBe("sub_newer");
  });

  it("breaks ties by latest period end for canceled subs too", () => {
    const olderCanceled = makeSub({
      id: "sub_old_cancel",
      status: "canceled",
      items: [{ current_period_end: 1_733_011_200 }],
    });
    const newerCanceled = makeSub({
      id: "sub_new_cancel",
      status: "canceled",
      items: [{ current_period_end: 1_735_689_600 }],
    });

    const result = selectBestSubscription([olderCanceled, newerCanceled]);
    expect(result?.id).toBe("sub_new_cancel");
  });

  it("handles mixed statuses with three subscriptions", () => {
    const expired = makeSub({
      id: "sub_expired",
      status: "incomplete_expired",
      items: [{ current_period_end: 1_735_689_600 }],
    });
    const canceled = makeSub({
      id: "sub_canceled",
      status: "canceled",
      items: [{ current_period_end: 1_735_689_600 }],
    });
    const active = makeSub({
      id: "sub_active",
      status: "active",
      items: [{ current_period_end: 1_733_011_200 }],
    });

    const result = selectBestSubscription([expired, canceled, active]);
    expect(result?.id).toBe("sub_active");
  });

  it("normalizes the selected subscription correctly", () => {
    const sub = makeSub({
      id: "sub_full",
      status: "active",
      cancel_at_period_end: true,
      trial_end: 1_738_368_000,
      items: [
        {
          current_period_end: 1_735_689_600,
          price: {
            id: "price_pro_yearly",
            lookup_key: "strait_pro_annual",
            recurring: { interval: "year" },
          },
        },
      ],
    });

    const result = selectBestSubscription([sub]);

    expect(result).toEqual({
      id: "sub_full",
      status: "active",
      productId: "price_pro_yearly",
      priceId: "price_pro_yearly",
      lookupKey: "strait_pro_annual",
      currentPeriodEnd: new Date("2025-01-01T00:00:00.000Z"),
      cancelAtPeriodEnd: true,
      recurringInterval: "year",
      trialEnd: new Date("2025-02-01T00:00:00.000Z"),
    });
  });

  it("handles subscriptions with no items gracefully", () => {
    const noItems = makeSub({
      id: "sub_no_items",
      status: "active",
      items: [],
    });
    const withItems = makeSub({
      id: "sub_with_items",
      status: "canceled",
      items: [{ current_period_end: 1_735_689_600, price: { id: "price_a" } }],
    });

    // Active (rank 0) still wins over canceled (rank 1) even without items
    const result = selectBestSubscription([noItems, withItems]);
    expect(result?.id).toBe("sub_no_items");
  });

  it("preserves order stability for identical subscriptions", () => {
    const sub1 = makeSub({
      id: "sub_first",
      status: "active",
      items: [{ current_period_end: 1_735_689_600 }],
    });
    const sub2 = makeSub({
      id: "sub_second",
      status: "active",
      items: [{ current_period_end: 1_735_689_600 }],
    });

    // When rank and period end are identical, the first one should win
    const result = selectBestSubscription([sub1, sub2]);
    expect(result?.id).toBe("sub_first");
  });
});
