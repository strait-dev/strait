import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";

/** A single anomaly alert flagging unusual spending patterns. */
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
    const orgId = getOrgIdFromSession(
      ctx.context.session as Record<string, unknown>
    );

    if (!orgId) {
      return [] as AnomalyAlert[];
    }

    return await runWithFallback(
      apiEffect<AnomalyAlert[]>("/v1/usage/anomalies", {
        params: { org_id: orgId },
      }),
      [] as AnomalyAlert[]
    );
  });

/** Query options for spending anomaly alerts. Refetches every 5 minutes. */
export const anomalyAlertsQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.anomalyAlerts.queryKey,
    queryFn: () => getAnomalyAlertsServerFn(),
    refetchInterval: 300_000,
  });
