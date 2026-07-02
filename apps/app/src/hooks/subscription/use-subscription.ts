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
import { PLAN_LOOKUP_KEYS } from "@strait/billing";
import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { Effect } from "effect";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getAuth } from "@/lib/auth.server";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { enforceRateLimit } from "@/lib/rate-limit.server";
import { findCustomerByOrg, getStripeClient } from "@/lib/stripe.server";
import { requireOrgAccess } from "@/middlewares/require-access";
import { selectBestSubscription } from "./subscription-helpers";
import {
  deriveSubscriptionState,
  type NormalizedSubscription,
  normalizePlanSlug,
  type PlanSlug,
  type SubscriptionData,
  type SubscriptionStateData,
} from "./subscription-state";

/** Maps stable Stripe lookup keys to plan slugs from the shared billing catalog. */
function getLookupKeyToPlan(): Map<string, PlanSlug> {
  return new Map(
    Object.entries(PLAN_LOOKUP_KEYS)
      .flatMap(([plan, keys]) => [
        [keys.monthly, plan],
        [keys.annual, plan],
      ])
      .filter((entry): entry is [string, PlanSlug] => Boolean(entry[0]))
  );
}

/**
 * Maps legacy Stripe Price IDs to plan slugs.
 *
 * Kept as a fallback for older subscriptions whose expanded Stripe Price
 * objects do not include a lookup_key.
 */
function getPriceToPlan(): Map<string, PlanSlug> {
  return new Map(
    [
      [process.env.STRIPE_STARTER_MONTHLY_PRICE_ID, "starter"],
      [process.env.STRIPE_STARTER_YEARLY_PRICE_ID, "starter"],
      [process.env.STRIPE_PRO_MONTHLY_PRICE_ID, "pro"],
      [process.env.STRIPE_PRO_YEARLY_PRICE_ID, "pro"],
      [process.env.STRIPE_SCALE_MONTHLY_PRICE_ID, "scale"],
      [process.env.STRIPE_SCALE_YEARLY_PRICE_ID, "scale"],
      [process.env.STRIPE_BUSINESS_MONTHLY_PRICE_ID, "business"],
      [process.env.STRIPE_BUSINESS_YEARLY_PRICE_ID, "business"],
      [process.env.STRIPE_ENTERPRISE_STARTER_YEARLY_PRICE_ID, "enterprise"],
      [process.env.STRIPE_ENTERPRISE_GROWTH_YEARLY_PRICE_ID, "enterprise"],
      [process.env.STRIPE_ENTERPRISE_LARGE_YEARLY_PRICE_ID, "enterprise"],
    ].filter((entry): entry is [string, PlanSlug] => Boolean(entry[0]))
  );
}

const subscriptionCache = new Map<
  string,
  { value: NormalizedSubscription | null; expiresAt: number }
>();
const subscriptionInflight = new Map<
  string,
  Promise<NormalizedSubscription | null>
>();

const SUBSCRIPTION_CACHE_MS = 60_000;

/**
 * Fetch the most relevant subscription for a customer by email.
 *
 * Looks up the Stripe customer, lists their subscriptions, and selects
 * the best one using {@link selectBestSubscription}.
 *
 * @returns The normalized subscription, or `null` if the customer has none.
 */
const getSubscriptionByEmail = async (
  email: string,
  orgId: string
): Promise<NormalizedSubscription | null> => {
  if (!process.env.STRIPE_SECRET_KEY) {
    return null;
  }

  const cacheKey = `${orgId}:${email}`;
  const cached = subscriptionCache.get(cacheKey);
  if (cached && cached.expiresAt > Date.now()) {
    return cached.value;
  }

  const inflight = subscriptionInflight.get(cacheKey);
  if (inflight) {
    return await inflight;
  }

  const request = (async () => {
    const customerId = await findCustomerByOrg(email, orgId);
    if (!customerId) {
      subscriptionCache.set(cacheKey, {
        value: null,
        expiresAt: Date.now() + SUBSCRIPTION_CACHE_MS,
      });
      return null;
    }

    const stripe = getStripeClient();
    const { data: subscriptions } = await stripe.subscriptions.list({
      customer: customerId,
      limit: 20,
      expand: ["data.items.data.price"],
    });

    const value = selectBestSubscription(subscriptions);
    subscriptionCache.set(cacheKey, {
      value,
      expiresAt: Date.now() + SUBSCRIPTION_CACHE_MS,
    });
    return value;
  })().finally(() => {
    subscriptionInflight.delete(cacheKey);
  });

  subscriptionInflight.set(cacheKey, request);
  return await request;
};

type SessionWithActiveOrg = {
  user: { id: string; email?: string | null };
  session: { activeOrganizationId?: string | null };
};

async function requireBillingSession(): Promise<{
  session: SessionWithActiveOrg;
  email: string;
  orgId: string;
}> {
  const headers = getRequestHeaders();
  const session = (await (
    await getAuth()
  ).api.getSession({
    headers,
  })) as SessionWithActiveOrg | null;
  const email = session?.user?.email;
  const orgId = session?.session?.activeOrganizationId;

  if (!(session && email && orgId)) {
    throw new Error("Unauthorized");
  }

  await requireOrgAccess(session.user.id, orgId);
  return { session, email, orgId };
}

/**
 * Server function: fetch the current user's raw subscription data from Stripe.
 */
const getSubscriptionServerFn = createServerFn({ method: "GET" }).handler(
  async (): Promise<SubscriptionData | null> => {
    let billingSession: Awaited<ReturnType<typeof requireBillingSession>>;
    try {
      billingSession = await requireBillingSession();
    } catch {
      return null;
    }

    await enforceRateLimit({
      key: `subscription:${billingSession.orgId}:${billingSession.session.user.id}`,
      limit: 30,
      windowSeconds: 60,
    });

    const subscription = await getSubscriptionByEmail(
      billingSession.email,
      billingSession.orgId
    );
    if (!subscription) {
      return null;
    }

    return {
      id: subscription.id,
      status: subscription.status,
      productId: subscription.productId,
      priceId: subscription.priceId,
      lookupKey: subscription.lookupKey,
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
  session: SessionWithActiveOrg | null
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
    let billingSession: Awaited<ReturnType<typeof requireBillingSession>>;
    try {
      billingSession = await requireBillingSession();
    } catch {
      return deriveSubscriptionState({
        subscription: null,
        planFromProduct: null,
        backendPlan: null,
      });
    }

    const subscription = await getSubscriptionByEmail(
      billingSession.email,
      billingSession.orgId
    );
    const backendPlan = await getBackendPlanTier(billingSession.session);

    let stripePlan: PlanSlug | null = null;
    if (subscription?.lookupKey) {
      stripePlan = getLookupKeyToPlan().get(subscription.lookupKey) ?? null;
    } else if (subscription?.priceId) {
      stripePlan = getPriceToPlan().get(subscription.priceId) ?? null;
    }
    const planFromProduct = normalizePlanSlug(stripePlan);

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
