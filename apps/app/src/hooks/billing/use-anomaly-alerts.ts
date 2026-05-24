/**
 * Spending anomaly alerts query hook.
 *
 * Fetches active anomaly alerts from `GET /v1/usage/anomalies`.
 * Anomaly alerts flag unusual spending patterns based on the org's
 * configured warning and critical thresholds.
 */

import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveOrgAccess } from "@/middlewares/require-access";
import { type AnomalySeverity, REFETCH_10M } from "./types";

/** A single anomaly alert flagging unusual spending patterns. */
export type AnomalyAlert = {
  /** Organization ID that triggered the alert. */
  org_id: string;
  /** Today's total spend in micro-USD. */
  today_spend: number;
  /** 7-day average daily spend in micro-USD. */
  avg_7d_spend: number;
  /** Ratio of today's spend to the 7-day average. */
  spike_ratio: number;
  /** Project contributing the most to the spike. */
  top_contributor: string;
  /** Alert severity level. */
  severity: AnomalySeverity;
};

/** Server function to fetch active anomaly alerts. */
const getAnomalyAlertsServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = await requireActiveOrgAccess(ctx.context);

    return await runWithSentryReport(
      apiEffect<AnomalyAlert[]>("/v1/usage/anomalies", {
        params: { org_id: orgId },
      })
    );
  });

/**
 * Query options for spending anomaly alerts.
 *
 * Refetches every 10 minutes.
 *
 * @returns TanStack Query options for `["billing", "anomalyAlerts"]`.
 */
export const anomalyAlertsQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.anomalyAlerts.queryKey,
    queryFn: () => getAnomalyAlertsServerFn(),
    refetchInterval: REFETCH_10M,
    refetchIntervalInBackground: false,
  });
