import { Polar } from "@polar-sh/sdk";
import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { auth } from "@/lib/auth";

type SubscriptionData = {
  id: string;
  status: string;
  productId: string;
  priceId: string;
  currentPeriodEnd: Date | null;
  cancelAtPeriodEnd: boolean;
};

type SubscriptionStateData = {
  planSlug: string | null;
  isTrialing: boolean;
  trialInfo: { trialEnd: Date | null } | null;
  hasActiveSubscription: boolean;
  status: string;
  subscription:
    | (SubscriptionData & { recurringInterval: string | null })
    | null;
  isActive: boolean;
  needsAttention: boolean;
  isCanceled: boolean;
  plan: string;
  nextPlan: { plan: string; name: string } | null;
  trialDaysLeft: number | null;
  shouldShowUpgrade: boolean;
  hasPendingPayment: boolean;
};

type NormalizedSubscription = SubscriptionData & {
  recurringInterval: string | null;
  trialEnd: Date | null;
};

const polarAccessToken = process.env.POLAR_ACCESS_TOKEN;

const polarClient = polarAccessToken
  ? new Polar({
      accessToken: polarAccessToken,
      server:
        (process.env.POLAR_SERVER as "sandbox" | "production") ?? "production",
    })
  : null;

const ACTIVE_STATUSES = new Set([
  "active",
  "trialing",
  "past_due",
  "incomplete",
  "unpaid",
]);
const ATTENTION_STATUSES = new Set(["past_due", "incomplete", "unpaid"]);
const CANCELED_STATUSES = new Set(["canceled", "cancelled"]);

const PRODUCT_PLAN_MAP: Record<string, string> = {
  [process.env.POLAR_STARTER_MONTHLY_ID ?? ""]: "starter",
  [process.env.POLAR_STARTER_YEARLY_ID ?? ""]: "starter",
  [process.env.POLAR_PROFESSIONAL_MONTHLY_ID ?? ""]: "professional",
  [process.env.POLAR_PROFESSIONAL_YEARLY_ID ?? ""]: "professional",
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

const toTitleCase = (value: string) =>
  value.charAt(0).toUpperCase() + value.slice(1);

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

      if (ACTIVE_STATUSES.has(status)) {
        rank = 0;
      } else if (CANCELED_STATUSES.has(status)) {
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
    const session = await auth.api.getSession({ headers });
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

const getSubscriptionStateServerFn = createServerFn({ method: "GET" }).handler(
  async (): Promise<SubscriptionStateData> => {
    const headers = getRequestHeaders();
    const session = await auth.api.getSession({ headers });
    const email = session?.user?.email;

    if (!email) {
      return {
        planSlug: null,
        isTrialing: false,
        trialInfo: null,
        hasActiveSubscription: false,
        status: "none",
        subscription: null,
        isActive: false,
        needsAttention: false,
        isCanceled: false,
        plan: "starter",
        nextPlan: null,
        trialDaysLeft: null,
        shouldShowUpgrade: true,
        hasPendingPayment: false,
      };
    }

    const subscription = await getSubscriptionByEmail(email);
    const status = subscription?.status ?? "none";
    const normalizedStatus = status === "cancelled" ? "canceled" : status;
    const planSlug = subscription?.productId
      ? (PRODUCT_PLAN_MAP[subscription.productId] ?? null)
      : null;
    const isTrialing = normalizedStatus === "trialing";
    const hasActiveSubscription = ACTIVE_STATUSES.has(normalizedStatus);
    const needsAttention = ATTENTION_STATUSES.has(normalizedStatus);
    const isCanceled = CANCELED_STATUSES.has(normalizedStatus);
    const trialEnd = subscription?.trialEnd ?? null;
    const trialDaysLeft = trialEnd
      ? Math.max(
          Math.ceil((trialEnd.getTime() - Date.now()) / (1000 * 60 * 60 * 24)),
          0
        )
      : null;
    const shouldShowUpgrade =
      !hasActiveSubscription || isTrialing || needsAttention || isCanceled;

    return {
      planSlug,
      isTrialing,
      trialInfo: isTrialing ? { trialEnd } : null,
      hasActiveSubscription,
      status: normalizedStatus,
      subscription: subscription
        ? {
            id: subscription.id,
            status: normalizedStatus,
            productId: subscription.productId,
            priceId: subscription.priceId,
            currentPeriodEnd: subscription.currentPeriodEnd,
            cancelAtPeriodEnd: subscription.cancelAtPeriodEnd,
            recurringInterval: subscription.recurringInterval,
          }
        : null,
      isActive: hasActiveSubscription && !isCanceled,
      needsAttention,
      isCanceled,
      plan: planSlug ?? "starter",
      nextPlan: planSlug
        ? {
            plan: planSlug,
            name: toTitleCase(planSlug),
          }
        : null,
      trialDaysLeft,
      shouldShowUpgrade,
      hasPendingPayment: needsAttention,
    };
  }
);

/** Query options for the current user's subscription details. */
export const subscriptionQueryOptions = () =>
  queryOptions({
    queryKey: ["subscription"],
    queryFn: () => getSubscriptionServerFn(),
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  });

/** Query options for the current subscription state (plan, limits, feature access). */
export const subscriptionStateQueryOptions = () =>
  queryOptions({
    queryKey: ["subscription", "state"],
    queryFn: () => getSubscriptionStateServerFn(),
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  });
