export type PlanSlug = "free" | "starter" | "pro" | "enterprise";

export type SubscriptionData = {
  id: string;
  status: string;
  productId: string;
  priceId: string;
  currentPeriodEnd: Date | null;
  cancelAtPeriodEnd: boolean;
};

export type SubscriptionStateData = {
  planSlug: PlanSlug | null;
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
  plan: PlanSlug;
  nextPlan: { plan: PlanSlug; name: string } | null;
  trialDaysLeft: number | null;
  shouldShowUpgrade: boolean;
  hasPendingPayment: boolean;
};

export type NormalizedSubscription = SubscriptionData & {
  recurringInterval: string | null;
  trialEnd: Date | null;
};

type DeriveSubscriptionStateInput = {
  subscription: NormalizedSubscription | null;
  planFromProduct: PlanSlug | null;
  backendPlan: PlanSlug | null;
  now?: number;
};

const ACTIVE_STATUSES = new Set([
  "active",
  "trialing",
  "past_due",
  "incomplete",
  "unpaid",
]);

const ATTENTION_STATUSES = new Set(["past_due", "incomplete", "unpaid"]);
const CANCELED_STATUSES = new Set(["canceled", "cancelled"]);

export const normalizePlanSlug = (value: string | null | undefined): PlanSlug | null => {
  switch (value) {
    case "free":
    case "starter":
    case "pro":
    case "enterprise":
      return value;
    default:
      return null;
  }
};

export const nextPlanFor = (
  plan: PlanSlug
): { plan: PlanSlug; name: string } | null => {
  switch (plan) {
    case "free":
      return { plan: "starter", name: "Starter" };
    case "starter":
      return { plan: "pro", name: "Pro" };
    case "pro":
      return { plan: "enterprise", name: "Enterprise" };
    case "enterprise":
      return null;
    default:
      return null;
  }
};

export const deriveSubscriptionState = ({
  subscription,
  planFromProduct,
  backendPlan,
  now = Date.now(),
}: DeriveSubscriptionStateInput): SubscriptionStateData => {
  const status = subscription?.status ?? "none";
  const normalizedStatus = status === "cancelled" ? "canceled" : status;
  const currentPlan = planFromProduct ?? backendPlan ?? "free";
  const isTrialing = normalizedStatus === "trialing";
  const hasActiveSubscription = ACTIVE_STATUSES.has(normalizedStatus);
  const needsAttention = ATTENTION_STATUSES.has(normalizedStatus);
  const isCanceled = CANCELED_STATUSES.has(normalizedStatus);
  const trialEnd = subscription?.trialEnd ?? null;
  const trialDaysLeft = trialEnd
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
    plan: currentPlan,
    nextPlan: nextPlanFor(currentPlan),
    trialDaysLeft,
    shouldShowUpgrade:
      !hasActiveSubscription || isTrialing || needsAttention || isCanceled,
    hasPendingPayment: needsAttention,
  };
};
