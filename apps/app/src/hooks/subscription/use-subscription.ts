import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { Effect } from "effect";
import Stripe from "stripe";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getAuth } from "@/lib/auth.server";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import {
  deriveSubscriptionState,
  type NormalizedSubscription,
  normalizePlanSlug,
  type PlanSlug,
  type SubscriptionData,
  type SubscriptionStateData,
} from "./subscription-state";

let _stripe: Stripe | null = null;

function getStripeClient(): Stripe | null {
  const key = process.env.STRIPE_SECRET_KEY;
  if (!key) {
    return null;
  }
  if (!_stripe) {
    _stripe = new Stripe(key, { apiVersion: "2025-08-27.basil" });
  }
  return _stripe;
}

const PRICE_PLAN_MAP: Record<string, string> = {
  [process.env.STRIPE_STARTER_MONTHLY_PRICE_ID ?? ""]: "starter",
  [process.env.STRIPE_STARTER_YEARLY_PRICE_ID ?? ""]: "starter",
  [process.env.STRIPE_PRO_MONTHLY_PRICE_ID ?? ""]: "pro",
  [process.env.STRIPE_PRO_YEARLY_PRICE_ID ?? ""]: "pro",
  [process.env.STRIPE_SCALE_MONTHLY_PRICE_ID ?? ""]: "scale",
  [process.env.STRIPE_SCALE_YEARLY_PRICE_ID ?? ""]: "scale",
};

const ACTIVE_STATUSES = new Set([
  "active",
  "trialing",
  "past_due",
  "incomplete",
  "unpaid",
]);

const getSubscriptionByEmail = async (
  email: string
): Promise<NormalizedSubscription | null> => {
  const stripe = getStripeClient();
  if (!stripe) {
    return null;
  }

  const customers = await stripe.customers.list({ email, limit: 1 });
  const customer = customers.data[0];
  if (!customer) {
    return null;
  }

  const subscriptions = await stripe.subscriptions.list({
    customer: customer.id,
    limit: 20,
    expand: ["data.items.data.price"],
  });

  if (subscriptions.data.length === 0) {
    return null;
  }

  // Rank subscriptions: active > canceled > other
  const ranked = subscriptions.data
    .map((sub) => {
      let rank = 2;
      if (ACTIVE_STATUSES.has(sub.status)) {
        rank = 0;
      } else if (sub.status === "canceled") {
        rank = 1;
      }

      const item = sub.items?.data?.[0];
      const periodEnd = item?.current_period_end
        ? new Date(item.current_period_end * 1000)
        : new Date(0);

      return { sub, rank, periodEnd };
    })
    .sort((a, b) => {
      if (a.rank !== b.rank) {
        return a.rank - b.rank;
      }
      return b.periodEnd.getTime() - a.periodEnd.getTime();
    });

  const selected = ranked[0]?.sub;
  if (!selected) {
    return null;
  }

  const item = selected.items?.data?.[0];
  const priceId = item?.price?.id ?? "";
  const recurringInterval = item?.price?.recurring?.interval ?? null;

  return {
    id: selected.id,
    status: selected.status,
    productId: priceId, // We use price ID as the product identifier
    priceId,
    currentPeriodEnd: item?.current_period_end
      ? new Date(item.current_period_end * 1000)
      : null,
    cancelAtPeriodEnd: selected.cancel_at_period_end,
    recurringInterval,
    trialEnd: selected.trial_end ? new Date(selected.trial_end * 1000) : null,
  };
};

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

/** Fetch the plan tier from the backend usage API as a fallback. */
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
      subscription?.productId ? PRICE_PLAN_MAP[subscription.productId] : null
    );

    return deriveSubscriptionState({
      subscription,
      planFromProduct,
      backendPlan,
    });
  }
);

/** Query options for the current user's subscription details. */
export const subscriptionQueryOptions = () =>
  queryOptions({
    queryKey: ["subscription"],
    queryFn: () => getSubscriptionServerFn(),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Query options for the current subscription state (plan, limits, feature access). */
export const subscriptionStateQueryOptions = () =>
  queryOptions({
    queryKey: ["subscription", "state"],
    queryFn: () => getSubscriptionStateServerFn(),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });
