import type {
  NotificationMessage,
  NotificationProvider,
  NotifyPolicyOverride,
  NotifySubscriber,
} from "@/hooks/api/types";

export type NotifyOpsHealth = "healthy" | "degraded" | "critical";

export type NotifyOpsTrend = "up" | "down" | "flat";

export type NotifyOpsRecommendation = {
  id:
    | "configure_default_provider"
    | "triage_failed_deliveries"
    | "review_unhealthy_provider"
    | "review_subscriber_health"
    | "review_suppression_events";
  label: string;
  description: string;
  to: string;
  search?: Record<string, string | string[] | undefined>;
};

export type NotifyOpsSnapshot = {
  totalDeliveries: number;
  failedDeliveries: number;
  bouncedDeliveries: number;
  suppressedDeliveries: number;
  pendingDeliveries: number;
  errorRate: number;
  avgDeliveryLatencySecs: number;
  recentErrorRate: number;
  previousErrorRate: number;
  errorRateTrend: NotifyOpsTrend;
  recentAvgLatencySecs: number;
  previousAvgLatencySecs: number;
  latencyTrend: NotifyOpsTrend;
  totalSubscribers: number;
  inactiveSubscribers: number;
  totalProviders: number;
  unhealthyProviders: number;
  hasDefaultEmailProvider: boolean;
  policyOverrides: number;
  health: NotifyOpsHealth;
  reasons: string[];
  recommendations: NotifyOpsRecommendation[];
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

const computeErrorRate = (deliveries: NotificationMessage[]) => {
  if (deliveries.length === 0) {
    return 0;
  }

  const failedCount = deliveries.filter(
    (item) => item.status === "failed" || item.status === "bounced"
  ).length;

  return failedCount / deliveries.length;
};

const computeAverageDeliveryLatencySecs = (
  deliveries: NotificationMessage[]
) => {
  const latencies = deliveries
    .filter((item) => !!item.delivered_at)
    .map((item) => {
      const createdAt = new Date(item.created_at).getTime();
      const deliveredAt = new Date(item.delivered_at as string).getTime();
      if (Number.isNaN(createdAt) || Number.isNaN(deliveredAt)) {
        return null;
      }
      return Math.max(0, (deliveredAt - createdAt) / 1000);
    })
    .filter((value): value is number => value !== null);

  if (latencies.length === 0) {
    return 0;
  }

  const total = latencies.reduce((sum, value) => sum + value, 0);
  return total / latencies.length;
};

const computeTrend = (recent: number, previous: number): NotifyOpsTrend => {
  const delta = recent - previous;
  if (Math.abs(delta) < 0.001) {
    return "flat";
  }
  return delta > 0 ? "up" : "down";
};

export const buildNotifyOpsSnapshot = ({
  deliveries,
  subscribers,
  providers,
  policies,
}: NotifyOpsSnapshotInput): NotifyOpsSnapshot => {
  const sortedDeliveries = [...deliveries].sort((a, b) =>
    b.created_at.localeCompare(a.created_at)
  );

  const failedDeliveries = sortedDeliveries.filter(
    (item) => item.status === "failed"
  ).length;
  const bouncedDeliveries = sortedDeliveries.filter(
    (item) => item.status === "bounced"
  ).length;
  const suppressedDeliveries = sortedDeliveries.filter(
    (item) => !!item.suppression_reason
  ).length;
  const pendingDeliveries = sortedDeliveries.filter((item) => {
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

  const totalDeliveries = sortedDeliveries.length;
  const totalSubscribers = subscribers.length;
  const totalProviders = providers.length;
  const policyOverrides = policies.length;

  const errorRate = computeErrorRate(sortedDeliveries);
  const avgDeliveryLatencySecs =
    computeAverageDeliveryLatencySecs(sortedDeliveries);

  const halfWindow = Math.max(1, Math.floor(sortedDeliveries.length / 2));
  const recentWindow = sortedDeliveries.slice(0, halfWindow);
  const previousWindow = sortedDeliveries.slice(halfWindow);

  const recentErrorRate = computeErrorRate(recentWindow);
  const previousErrorRate = computeErrorRate(previousWindow);
  const errorRateTrend = computeTrend(recentErrorRate, previousErrorRate);

  const recentAvgLatencySecs = computeAverageDeliveryLatencySecs(recentWindow);
  const previousAvgLatencySecs =
    computeAverageDeliveryLatencySecs(previousWindow);
  const latencyTrend = computeTrend(
    recentAvgLatencySecs,
    previousAvgLatencySecs
  );

  const reasons: string[] = [];
  const recommendations: NotifyOpsRecommendation[] = [];

  if (!hasDefaultEmailProvider) {
    reasons.push("No default email provider is configured.");
    recommendations.push({
      id: "configure_default_provider",
      label: "Configure a default email provider",
      description:
        "Set a default SES provider so email deliveries always have a fallback route.",
      to: "/app/notify/providers",
    });
  }

  if (errorRate >= 0.2) {
    reasons.push("Delivery error rate is above 20% in the current window.");
  } else if (errorRate >= 0.05) {
    reasons.push("Delivery error rate is above 5% in the current window.");
  }

  if (errorRate >= 0.05) {
    recommendations.push({
      id: "triage_failed_deliveries",
      label: "Triage failed deliveries",
      description:
        "Review failed and bounced deliveries, then inspect suppression and provider responses.",
      to: "/app/notify/deliveries",
      search: { status: ["failed", "bounced"] },
    });
  }

  if (suppressedDeliveries > 0) {
    recommendations.push({
      id: "review_suppression_events",
      label: "Review suppression candidates",
      description:
        "Inspect suppressed deliveries and guide operators to subscriber unsuppress controls.",
      to: "/app/notify/deliveries",
      search: { query: "suppressed" },
    });
  }

  if (unhealthyProviders > 0) {
    reasons.push("At least one provider is reporting non-healthy state.");
    recommendations.push({
      id: "review_unhealthy_provider",
      label: "Review provider health",
      description:
        "Check provider configuration, region, and fallback strategy for degraded providers.",
      to: "/app/notify/providers",
    });
  }

  if (totalSubscribers >= 10 && inactiveSubscribers / totalSubscribers > 0.5) {
    reasons.push("More than half of subscribers are not active.");
    recommendations.push({
      id: "review_subscriber_health",
      label: "Review subscriber status distribution",
      description:
        "Assess inactive subscribers and preference/suppression patterns.",
      to: "/app/notify/subscribers",
    });
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
    avgDeliveryLatencySecs,
    recentErrorRate,
    previousErrorRate,
    errorRateTrend,
    recentAvgLatencySecs,
    previousAvgLatencySecs,
    latencyTrend,
    totalSubscribers,
    inactiveSubscribers,
    totalProviders,
    unhealthyProviders,
    hasDefaultEmailProvider,
    policyOverrides,
    health,
    reasons,
    recommendations,
  };
};
