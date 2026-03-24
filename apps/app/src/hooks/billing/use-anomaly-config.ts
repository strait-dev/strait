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
import { getOrgIdFromSession } from "./session";

export type AnomalyConfigData = {
  warning_threshold: number;
  critical_threshold: number;
};

const getAnomalyConfigServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = getOrgIdFromSession(
      ctx.context.session as Record<string, unknown>
    );

    if (!orgId) {
      return null;
    }

    return await runWithSentryReport(
      apiEffect<AnomalyConfigData>("/v1/anomaly-config", {
        params: { org_id: orgId },
      })
    );
  });

export const anomalyConfigQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.anomalyConfig.queryKey,
    queryFn: () => getAnomalyConfigServerFn(),
    refetchInterval: 600_000,
    refetchIntervalInBackground: false,
  });

type SetConfigInput = {
  warningThreshold: number;
  criticalThreshold: number;
};

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
    const orgId = getOrgIdFromSession(
      context.session as Record<string, unknown>
    );

    if (!orgId) {
      throw new Error("No organization found");
    }

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

export function useSetAnomalyConfig() {
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
}
