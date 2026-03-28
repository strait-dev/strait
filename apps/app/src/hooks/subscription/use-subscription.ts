import { Polar } from "@polar-sh/sdk";
import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { Effect } from "effect";
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

const disablePolarBilling = process.env.DISABLE_POLAR_BILLING === "true";
const polarAccessToken = process.env.POLAR_ACCESS_TOKEN;

const polarClient =
  !disablePolarBilling && polarAccessToken
    ? new Polar({
        accessToken: polarAccessToken,
        server:
          (process.env.POLAR_SERVER as "sandbox" | "production") ??
          "production",
      })
    : null;

const PRODUCT_PLAN_MAP: Record<string, string> = disablePolarBilling
  ? {}
  : {
      [process.env.POLAR_STARTER_MONTHLY_ID ?? ""]: "starter",
      [process.env.POLAR_STARTER_YEARLY_ID ?? ""]: "starter",
      [process.env.POLAR_PRO_MONTHLY_ID ?? ""]: "pro",
      [process.env.POLAR_PRO_YEARLY_ID ?? ""]: "pro",
    };

const toRecord = (value: unknown): Record<string, unknown> | null =>
  value && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;

const toDate = (value: unknown): Date | null => {
  if (!value) {
    return null;
  }

  if (value instanceof Date) {
    return value;
  }

  const date = new Date(String(value));
  return Number.isNaN(date.getTime()) ? null : date;
};

const asString = (value: unknown): string =>
  typeof value === "string" ? value : "";

const asBoolean = (value: unknown): boolean =>
  typeof value === "boolean" ? value : false;

const getSubscriptionByEmail = async (
  email: string
): Promise<NormalizedSubscription | null> => {
  if (!polarClient) {
    return null;
  }

  const { result: customersResult } = await polarClient.customers.list({
    email,
    limit: 1,
  });

  const customers = customersResult.items;
  const customer = Array.isArray(customers) ? customers[0] : null;

  if (!customer) {
    return null;
  }

  const { result: subscriptionsResult } = await polarClient.subscriptions.list({
    customerId: customer.id,
    limit: 20,
  });

  const items = Array.isArray(subscriptionsResult.items)
    ? subscriptionsResult.items
    : [];

  const ranked = items
    .map((item) => {
      const status = asString(toRecord(item)?.status);
      let rank = 2;

      if (
        status === "active" ||
        status === "trialing" ||
        status === "past_due" ||
        status === "incomplete" ||
        status === "unpaid"
      ) {
        rank = 0;
      } else if (status === "canceled" || status === "cancelled") {
        rank = 1;
      }

      return {
        item,
        rank,
        periodEnd:
          toDate(toRecord(item)?.currentPeriodEnd) ??
          toDate(toRecord(item)?.currentPeriodEndAt) ??
          new Date(0),
      };
    })
    .sort((a, b) => {
      if (a.rank !== b.rank) {
        return a.rank - b.rank;
      }

      return b.periodEnd.getTime() - a.periodEnd.getTime();
    });

  const selected = ranked[0]?.item;
  const record = toRecord(selected);

  if (!record) {
    return null;
  }

  const product = toRecord(record.product);
  const price = toRecord(record.price);

  return {
    id: asString(record.id),
    status: asString(record.status),
    productId: asString(record.productId || product?.id),
    priceId: asString(record.priceId || price?.id),
    currentPeriodEnd:
      toDate(record.currentPeriodEnd) ?? toDate(record.currentPeriodEndAt),
    cancelAtPeriodEnd: asBoolean(record.cancelAtPeriodEnd),
    recurringInterval:
      asString(
        record.recurringInterval || record.interval || price?.recurringInterval
      ) || null,
    trialEnd: toDate(record.trialEnd) ?? toDate(record.trialEndsAt),
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
      subscription?.productId ? PRODUCT_PLAN_MAP[subscription.productId] : null
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
