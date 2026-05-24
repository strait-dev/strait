/**
 * Subscription data fetching hooks for the Strait billing system.
 *
 * Fetches subscription state from two sources:
 * 1. **Stripe API** -- the source of truth for subscription status, plan, and billing period.
 * 2. **Go backend** (`/v1/usage/current`) -- fallback for plan tier when Stripe is unavailable.
 *
 * The derived state ({@link subscriptionStateQueryOptions}) combines both sources
 * and is used throughout the app for feature gating, upgrade prompts, and billing UI.
 *
 * @see https://docs.stripe.com/api/subscriptions — Stripe Subscriptions API
 */
import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { Effect } from "effect";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getAuth } from "@/lib/auth.server";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { findCustomerByEmail, getStripeClient } from "@/lib/stripe.server";
import { selectBestSubscription } from "./subscription-helpers";
import {
  deriveSubscriptionState,
  type NormalizedSubscription,
  normalizePlanSlug,
  type PlanSlug,
  type SubscriptionData,
  type SubscriptionStateData,
} from "./subscription-state";

/**
 * Maps Stripe Price IDs to plan slugs.
 *
 * Each plan tier has two prices (monthly + yearly) that both resolve
 * to the same slug. Populated from env vars set via Infisical.
 */
const PRICE_TO_PLAN = new Map<string, PlanSlug>([
  [process.env.STRIPE_STARTER_MONTHLY_PRICE_ID ?? "", "starter"],
  [process.env.STRIPE_STARTER_YEARLY_PRICE_ID ?? "", "starter"],
  [process.env.STRIPE_PRO_MONTHLY_PRICE_ID ?? "", "pro"],
  [process.env.STRIPE_PRO_YEARLY_PRICE_ID ?? "", "pro"],
  [process.env.STRIPE_SCALE_MONTHLY_PRICE_ID ?? "", "scale"],
  [process.env.STRIPE_SCALE_YEARLY_PRICE_ID ?? "", "scale"],
  [process.env.STRIPE_ENTERPRISE_STARTER_YEARLY_PRICE_ID ?? "", "enterprise"],
  [process.env.STRIPE_ENTERPRISE_GROWTH_YEARLY_PRICE_ID ?? "", "enterprise"],
  [process.env.STRIPE_ENTERPRISE_LARGE_YEARLY_PRICE_ID ?? "", "enterprise"],
]);

/**
 * Fetch the most relevant subscription for a customer by email.
 *
 * Looks up the Stripe customer, lists their subscriptions, and selects
 * the best one using {@link selectBestSubscription}.
 *
 * @returns The normalized subscription, or `null` if the customer has none.
 */
const getSubscriptionByEmail = async (
  email: string
): Promise<NormalizedSubscription | null> => {
  if (!process.env.STRIPE_SECRET_KEY) {
    return null;
  }

  const customerId = await findCustomerByEmail(email);
  if (!customerId) {
    return null;
  }

  const stripe = getStripeClient();
  const { data: subscriptions } = await stripe.subscriptions.list({
    customer: customerId,
    limit: 20,
    expand: ["data.items.data.price"],
  });

  return selectBestSubscription(subscriptions);
};

/**
 * Server function: fetch the current user's raw subscription data from Stripe.
 */
const getSubscriptionServerFn = createServerFn({ method: "GET" }).handler(
  async (): Promise<SubscriptionData | null> => {
    const headers = getRequestHeaders();
    const session = await (await getAuth()).api.getSession({ headers });
    const email = session?.user?.email;

    if (!email) {
      return null;
    }

    const subscription = await getSubscriptionByEmail(email);
    if (!subscription) {
      return null;
    }

    return {
      id: subscription.id,
      status: subscription.status,
      productId: subscription.productId,
      priceId: subscription.priceId,
      currentPeriodEnd: subscription.currentPeriodEnd,
      cancelAtPeriodEnd: subscription.cancelAtPeriodEnd,
    };
  }
);

/**
 * Fetch the plan tier from the Go backend usage API.
 * Used as a fallback when the Stripe price-to-plan mapping doesn't resolve
 * (e.g. enterprise plans with custom pricing not in {@link PRICE_TO_PLAN}).
 */
const getBackendPlanTier = async (
  session: { session: { activeOrganizationId?: string | null } } | null
): Promise<PlanSlug | null> => {
  const orgId = session?.session?.activeOrganizationId;
  if (!orgId) {
    return null;
  }
  return await runWithFallback(
    apiEffect<{ plan: string }>("/v1/usage/current", {
      params: { org_id: orgId },
    }).pipe(Effect.map((data) => normalizePlanSlug(data?.plan ?? null))),
    null
  );
};

/**
 * Server function: derive the full subscription state for the current user.
 *
 * Combines Stripe subscription data with the backend plan tier fallback
 * to produce the {@link SubscriptionStateData} used throughout the app.
 */
const getSubscriptionStateServerFn = createServerFn({ method: "GET" }).handler(
  async (): Promise<SubscriptionStateData> => {
    const headers = getRequestHeaders();
    const session = await (await getAuth()).api.getSession({ headers });
    const email = session?.user?.email;

    if (!email) {
      return deriveSubscriptionState({
        subscription: null,
        planFromProduct: null,
        backendPlan: null,
      });
    }

    const subscription = await getSubscriptionByEmail(email);
    const backendPlan = await getBackendPlanTier(session);

    const planFromProduct = normalizePlanSlug(
      subscription?.productId
        ? (PRICE_TO_PLAN.get(subscription.productId) ?? null)
        : null
    );

    return deriveSubscriptionState({
      subscription,
      planFromProduct,
      backendPlan,
    });
  }
);

/** Query options for the current user's raw subscription details. */
export const subscriptionQueryOptions = () =>
  queryOptions({
    queryKey: ["subscription"],
    queryFn: () => getSubscriptionServerFn(),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/**
 * Query options for the current user's derived subscription state.
 * This is the primary hook used by billing UI, feature gates, and upgrade prompts.
 */
export const subscriptionStateQueryOptions = () =>
  queryOptions({
    queryKey: ["subscription", "state"],
    queryFn: () => getSubscriptionStateServerFn(),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });
