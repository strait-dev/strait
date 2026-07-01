/**
 * Pure helper functions for subscription data normalization and ranking.
 *
 * Extracted from use-subscription.ts so they can be unit tested
 * without mocking Stripe API calls or server function context.
 */
import type Stripe from "stripe";
import type { NormalizedSubscription } from "./subscription-state";

/**
 * Subscription ranking priority for selecting the "best" subscription
 * when a customer has multiple (e.g. one active, one canceled).
 *
 * Lower rank = higher priority. Ties are broken by most recent period end.
 */
export const SUBSCRIPTION_RANK: Record<string, number> = {
  active: 0,
  trialing: 0,
  past_due: 0,
  incomplete: 0,
  unpaid: 0,
  canceled: 1,
  paused: 1,
  incomplete_expired: 2,
};

export const DEFAULT_RANK = 3;

/**
 * Extract the first subscription item from a Stripe subscription.
 * Returns `null` if the subscription has no items (shouldn't happen in practice).
 */
export const getFirstItem = (
  sub: Stripe.Subscription
): Stripe.SubscriptionItem | null => sub.items?.data?.[0] ?? null;

/**
 * Convert a Unix timestamp (seconds) to a `Date`, or `null` if zero/undefined.
 */
export const fromUnix = (ts: number | null | undefined): Date | null =>
  ts ? new Date(ts * 1000) : null;

/**
 * Normalize a Stripe subscription into the app's {@link NormalizedSubscription} shape.
 */
export const toNormalizedSubscription = (
  sub: Stripe.Subscription
): NormalizedSubscription => {
  const item = getFirstItem(sub);
  const priceId = item?.price?.id ?? "";
  const lookupKey = item?.price?.lookup_key ?? "";

  return {
    id: sub.id,
    status: sub.status,
    productId: priceId,
    priceId,
    lookupKey,
    currentPeriodEnd: fromUnix(item?.current_period_end),
    cancelAtPeriodEnd: sub.cancel_at_period_end,
    recurringInterval: item?.price?.recurring?.interval ?? null,
    trialEnd: fromUnix(sub.trial_end),
  };
};

/**
 * Select the best subscription from a list of Stripe subscriptions.
 *
 * Ranking:
 * 1. Active/trialing/past_due/incomplete/unpaid (rank 0)
 * 2. Canceled/paused (rank 1)
 * 3. Expired (rank 2)
 *
 * Within the same rank, the subscription with the latest billing period end wins.
 *
 * @returns The normalized best subscription, or `null` if the list is empty.
 */
export const selectBestSubscription = (
  subscriptions: Stripe.Subscription[]
): NormalizedSubscription | null => {
  if (subscriptions.length === 0) {
    return null;
  }

  // Single subscription fast path (most common case).
  if (subscriptions.length === 1) {
    return toNormalizedSubscription(subscriptions[0]);
  }

  // Multiple subscriptions: rank and pick the best one.
  const best = subscriptions
    .map((sub) => {
      const item = getFirstItem(sub);
      const periodEnd = fromUnix(item?.current_period_end)?.getTime() ?? 0;
      const rank = SUBSCRIPTION_RANK[sub.status] ?? DEFAULT_RANK;
      return { sub, rank, periodEnd };
    })
    .sort((a, b) =>
      a.rank === b.rank ? b.periodEnd - a.periodEnd : a.rank - b.rank
    )[0];

  return best ? toNormalizedSubscription(best.sub) : null;
};
