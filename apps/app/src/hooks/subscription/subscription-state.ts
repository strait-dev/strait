/**
 * Subscription state derivation for the Strait billing system.
 *
 * This module takes raw subscription data (from Stripe via {@link NormalizedSubscription})
 * and derives the UI-facing state used throughout the app: plan tier, feature access,
 * upgrade prompts, trial info, and payment status.
 *
 * Pure functions only -- no API calls, no side effects, fully testable.
 */

/** The five plan tiers in Strait, ordered by feature level. */
export type PlanSlug =
  | "free"
  | "starter"
  | "pro"
  | "scale"
  | "business"
  | "enterprise";

/**
 * Stripe subscription status values.
 *
 * @see https://docs.stripe.com/api/subscriptions/object#subscription_object-status
 */
export type SubscriptionStatus =
  | "active"
  | "canceled"
  | "incomplete"
  | "incomplete_expired"
  | "past_due"
  | "paused"
  | "trialing"
  | "unpaid"
  | "none";

/** Raw subscription data as returned by the Stripe lookup in use-subscription.ts. */
export type SubscriptionData = {
  id: string;
  status: string;
  productId: string;
  priceId: string;
  lookupKey: string;
  currentPeriodEnd: Date | null;
  cancelAtPeriodEnd: boolean;
};

/**
 * Extended subscription data with recurring interval and trial info.
 * This is the shape returned by {@link getSubscriptionByEmail} in use-subscription.ts.
 */
export type NormalizedSubscription = SubscriptionData & {
  recurringInterval: string | null;
  trialEnd: Date | null;
};

/**
 * The derived subscription state consumed by UI components.
 *
 * This is the single source of truth for:
 * - Which plan the user is on ({@link plan})
 * - Whether they have an active subscription ({@link isActive})
 * - Whether to show upgrade prompts ({@link shouldShowUpgrade})
 * - Trial status ({@link isTrialing}, {@link trialDaysLeft})
 * - Payment issues ({@link needsAttention}, {@link hasPendingPayment})
 */
export type SubscriptionStateData = {
  /** The resolved plan slug (Stripe price mapping > backend fallback > "free"). */
  planSlug: PlanSlug;
  /** Whether the subscription is in a trialing state. */
  isTrialing: boolean;
  /** Trial details, only present when {@link isTrialing} is true. */
  trialInfo: { trialEnd: Date | null } | null;
  /** Whether the subscription has any active-like status (active, trialing, past_due, etc.). */
  hasActiveSubscription: boolean;
  /** The normalized subscription status string. */
  status: SubscriptionStatus;
  /** The full subscription object with interval, or null if no subscription exists. */
  subscription:
    | (SubscriptionData & { recurringInterval: string | null })
    | null;
  /** True when the user has a usable subscription (active and not canceled). */
  isActive: boolean;
  /** True when payment issues need user action (past_due, incomplete, unpaid). */
  needsAttention: boolean;
  /** True when the subscription has been canceled (may still be active until period end). */
  isCanceled: boolean;
  /** Convenience alias for {@link planSlug}. Used in feature gate checks. */
  plan: PlanSlug;
  /** The next upgrade tier, or null if already on enterprise. */
  nextPlan: { plan: PlanSlug; name: string } | null;
  /** Days remaining in trial, or null if not trialing. */
  trialDaysLeft: number | null;
  /** Whether the upgrade banner/CTA should be shown. */
  shouldShowUpgrade: boolean;
  /** Whether there is a pending payment that needs resolution. */
  hasPendingPayment: boolean;
};

/** Subscription statuses that represent a usable (non-canceled, non-expired) subscription. */
const ACTIVE_STATUSES = new Set<SubscriptionStatus>([
  "active",
  "trialing",
  "past_due",
  "incomplete",
  "unpaid",
]);

/** Statuses where the user needs to take action (payment issue). */
const ATTENTION_STATUSES = new Set<SubscriptionStatus>([
  "past_due",
  "incomplete",
  "unpaid",
]);

/** Statuses representing a canceled subscription. */
const CANCELED_STATUSES = new Set<SubscriptionStatus>(["canceled"]);

/** Plan display names for the next-plan upgrade CTA. */
const PLAN_DISPLAY_NAMES: Record<PlanSlug, string> = {
  free: "Free",
  starter: "Starter",
  pro: "Pro",
  scale: "Scale",
  business: "Business",
  enterprise: "Enterprise",
};

/**
 * Validate and normalize a plan slug string.
 *
 * @returns The typed {@link PlanSlug} if valid, or `null` for unrecognized values.
 */
export const normalizePlanSlug = (
  value: string | null | undefined
): PlanSlug | null => {
  switch (value) {
    case "free":
    case "starter":
    case "pro":
    case "scale":
    case "business":
    case "enterprise":
      return value;
    default:
      return null;
  }
};

/**
 * Get the next upgrade tier for a given plan.
 *
 * @returns The next plan slug and display name, or `null` if already at the top (enterprise).
 */
export const nextPlanFor = (
  plan: PlanSlug
): { plan: PlanSlug; name: string } | null => {
  switch (plan) {
    case "free":
      return { plan: "starter", name: PLAN_DISPLAY_NAMES.starter };
    case "starter":
      return { plan: "pro", name: PLAN_DISPLAY_NAMES.pro };
    case "pro":
      return { plan: "scale", name: PLAN_DISPLAY_NAMES.scale };
    case "scale":
      return { plan: "business", name: PLAN_DISPLAY_NAMES.business };
    case "business":
      return { plan: "enterprise", name: PLAN_DISPLAY_NAMES.enterprise };
    case "enterprise":
      return null;
    default:
      return null;
  }
};

/** Input to {@link deriveSubscriptionState}. */
type DeriveSubscriptionStateInput = {
  /** The user's normalized Stripe subscription, or null if none. */
  subscription: NormalizedSubscription | null;
  /** The plan derived from the Stripe Price ID mapping. */
  planFromProduct: PlanSlug | null;
  /** The plan from the Go backend usage API (fallback). */
  backendPlan: PlanSlug | null;
  /** Override for Date.now() in tests. */
  now?: number;
};

/**
 * Derive the full subscription state from raw inputs.
 *
 * Resolution order for the plan:
 * 1. Stripe lookup-key or price-to-plan mapping ({@link planFromProduct})
 * 2. Go backend usage API ({@link backendPlan})
 * 3. Default to `"free"`
 *
 * @returns The derived {@link SubscriptionStateData} for UI consumption.
 */
export const deriveSubscriptionState = ({
  subscription,
  planFromProduct,
  backendPlan,
  now = Date.now(),
}: DeriveSubscriptionStateInput): SubscriptionStateData => {
  const rawStatus = subscription?.status ?? "none";
  const status = (
    rawStatus === "cancelled" ? "canceled" : rawStatus
  ) as SubscriptionStatus;
  const currentPlan = planFromProduct ?? backendPlan ?? "free";

  const isTrialing = status === "trialing";
  const hasActiveSubscription = ACTIVE_STATUSES.has(status);
  const needsAttention = ATTENTION_STATUSES.has(status);
  const isCanceled = CANCELED_STATUSES.has(status);

  const trialEnd = subscription?.trialEnd ?? null;
  const trialDaysLeft =
    isTrialing && trialEnd
      ? Math.max(
          Math.ceil((trialEnd.getTime() - now) / (1000 * 60 * 60 * 24)),
          0
        )
      : null;

  return {
    planSlug: currentPlan,
    isTrialing,
    trialInfo: isTrialing ? { trialEnd } : null,
    hasActiveSubscription,
    status,
    subscription: subscription
      ? {
          id: subscription.id,
          status,
          productId: subscription.productId,
          priceId: subscription.priceId,
          lookupKey: subscription.lookupKey,
          currentPeriodEnd: subscription.currentPeriodEnd,
          cancelAtPeriodEnd: subscription.cancelAtPeriodEnd,
          recurringInterval: subscription.recurringInterval,
        }
      : null,
    isActive: hasActiveSubscription && !isCanceled,
    needsAttention,
    isCanceled,
    plan: currentPlan,
    nextPlan: nextPlanFor(currentPlan),
    trialDaysLeft,
    shouldShowUpgrade:
      !hasActiveSubscription || isTrialing || needsAttention || isCanceled,
    hasPendingPayment: needsAttention,
  };
};
