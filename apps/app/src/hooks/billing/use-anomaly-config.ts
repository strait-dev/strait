/**
 * Anomaly detection configuration query and mutation hooks.
 *
 * Fetches and updates the organization's spending anomaly detection
 * thresholds from `GET/PUT /v1/anomaly-config`.
 */

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import {
  requireActiveOrgAccess,
  requireActiveOrgAdmin,
} from "@/middlewares/require-access";
import { REFETCH_10M } from "./types";

/** Anomaly detection threshold configuration for the organization. */
export type AnomalyConfigData = {
  /** Spend-to-average ratio that triggers a warning alert. */
  warning_threshold: number;
  /** Spend-to-average ratio that triggers a critical alert. */
  critical_threshold: number;
};

/** Server function to fetch the anomaly detection configuration. */
const getAnomalyConfigServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = await requireActiveOrgAccess(ctx.context);

    return await runWithSentryReport(
      apiEffect<AnomalyConfigData>("/v1/anomaly-config", {
        params: { org_id: orgId },
      })
    );
  });

/**
 * Query options for the organization's anomaly detection thresholds.
 *
 * Refetches every 10 minutes.
 *
 * @returns TanStack Query options for `["billing", "anomalyConfig"]`.
 */
export const anomalyConfigQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.anomalyConfig.queryKey,
    queryFn: () => getAnomalyConfigServerFn(),
    refetchInterval: REFETCH_10M,
    refetchIntervalInBackground: false,
  });

/** Input for the anomaly config update mutation. */
type SetConfigInput = {
  warningThreshold: number;
  criticalThreshold: number;
};

/** Server function to update the anomaly detection thresholds. */
const setAnomalyConfigServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: SetConfigInput) =>
    z
      .object({
        warningThreshold: z.number().gt(1),
        criticalThreshold: z.number().gt(1),
      })
      .refine((d) => d.criticalThreshold > d.warningThreshold, {
        message: "Critical threshold must be greater than warning threshold",
      })
      .parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    const orgId = await requireActiveOrgAdmin(context);

    return await runWithSentryReport(
      apiEffect<{ status: string }>("/v1/anomaly-config", {
        method: "PUT",
        params: { org_id: orgId },
        body: {
          warning_threshold: data.warningThreshold,
          critical_threshold: data.criticalThreshold,
        },
      })
    );
  });

/**
 * Mutation hook for updating the anomaly detection thresholds.
 *
 * Invalidates both the anomaly config and anomaly alerts queries on settlement.
 *
 * @returns A TanStack Query mutation for anomaly config updates.
 */
export const useSetAnomalyConfig = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (params: SetConfigInput) =>
      setAnomalyConfigServerFn({ data: params }),
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.billing.anomalyConfig.queryKey,
      });
      queryClient.invalidateQueries({
        queryKey: queryKeys.billing.anomalyAlerts.queryKey,
      });
    },
  });
};
