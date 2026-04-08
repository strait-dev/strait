import type {
  NotificationMessage,
  NotificationProvider,
  NotifyPolicyOverride,
  NotifySubscriber,
} from "@/hooks/api/types";

export type NotifyOpsHealth = "healthy" | "degraded" | "critical";

export type NotifyOpsSnapshot = {
  totalDeliveries: number;
  failedDeliveries: number;
  bouncedDeliveries: number;
  suppressedDeliveries: number;
  pendingDeliveries: number;
  errorRate: number;
  totalSubscribers: number;
  inactiveSubscribers: number;
  totalProviders: number;
  unhealthyProviders: number;
  hasDefaultEmailProvider: boolean;
  policyOverrides: number;
  health: NotifyOpsHealth;
  reasons: string[];
};

type NotifyOpsSnapshotInput = {
  deliveries: NotificationMessage[];
  subscribers: NotifySubscriber[];
  providers: NotificationProvider[];
  policies: NotifyPolicyOverride[];
};

const isHealthyProviderState = (health: string) => {
  const normalized = health.toLowerCase();
  return normalized === "healthy" || normalized === "ok";
};

export const buildNotifyOpsSnapshot = ({
  deliveries,
  subscribers,
  providers,
  policies,
}: NotifyOpsSnapshotInput): NotifyOpsSnapshot => {
  const failedDeliveries = deliveries.filter(
    (item) => item.status === "failed"
  ).length;
  const bouncedDeliveries = deliveries.filter(
    (item) => item.status === "bounced"
  ).length;
  const suppressedDeliveries = deliveries.filter(
    (item) => !!item.suppression_reason
  ).length;
  const pendingDeliveries = deliveries.filter((item) => {
    const status = item.status.toLowerCase();
    return (
      status === "pending" || status === "processing" || status === "scheduled"
    );
  }).length;

  const inactiveSubscribers = subscribers.filter(
    (item) => item.status.toLowerCase() !== "active"
  ).length;

  const unhealthyProviders = providers.filter(
    (item) => !isHealthyProviderState(item.health || "")
  ).length;

  const hasDefaultEmailProvider = providers.some(
    (item) => item.channel.toLowerCase() === "email" && item.is_default
  );

  const totalDeliveries = deliveries.length;
  const totalSubscribers = subscribers.length;
  const totalProviders = providers.length;
  const policyOverrides = policies.length;

  const errorRate =
    totalDeliveries === 0
      ? 0
      : (failedDeliveries + bouncedDeliveries) / totalDeliveries;

  const reasons: string[] = [];

  if (!hasDefaultEmailProvider) {
    reasons.push("No default email provider is configured.");
  }

  if (errorRate >= 0.2) {
    reasons.push("Delivery error rate is above 20% in the current window.");
  } else if (errorRate >= 0.05) {
    reasons.push("Delivery error rate is above 5% in the current window.");
  }

  if (unhealthyProviders > 0) {
    reasons.push("At least one provider is reporting non-healthy state.");
  }

  if (totalSubscribers >= 10 && inactiveSubscribers / totalSubscribers > 0.5) {
    reasons.push("More than half of subscribers are not active.");
  }

  let health: NotifyOpsHealth = "healthy";
  if (!hasDefaultEmailProvider || errorRate >= 0.2) {
    health = "critical";
  } else if (reasons.length > 0) {
    health = "degraded";
  }

  if (reasons.length === 0) {
    reasons.push("All notify operational checks are healthy.");
  }

  return {
    totalDeliveries,
    failedDeliveries,
    bouncedDeliveries,
    suppressedDeliveries,
    pendingDeliveries,
    errorRate,
    totalSubscribers,
    inactiveSubscribers,
    totalProviders,
    unhealthyProviders,
    hasDefaultEmailProvider,
    policyOverrides,
    health,
    reasons,
  };
};
