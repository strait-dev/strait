import { useQuery } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

export type AnomalyAlert = {
  org_id: string;
  today_spend: number;
  avg_7d_spend: number;
  spike_ratio: number;
  top_contributor: string;
  severity: "warning" | "high" | "critical";
};

const getAnomalyAlertsServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = (ctx.context.session as Record<string, unknown>)
      .activeOrganizationId;

    if (!orgId || typeof orgId !== "string") {
      return [] as AnomalyAlert[];
    }

    return await runWithFallback(
      apiEffect<AnomalyAlert[]>("/v1/usage/anomalies", {
        params: { org_id: orgId },
      }),
      [] as AnomalyAlert[]
    );
  });

export function useAnomalyAlerts() {
  return useQuery({
    queryKey: ["anomaly-alerts"],
    queryFn: () => getAnomalyAlertsServerFn(),
    refetchInterval: 300_000,
  });
}
