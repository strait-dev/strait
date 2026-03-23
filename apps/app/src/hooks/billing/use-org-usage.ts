import { queryOptions, useQuery } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import {
  EMPTY_ORG_USAGE,
  normalizeOrgUsageData,
  type RawOrgUsageData,
} from "@/hooks/billing/org-usage";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";

const getOrgUsageServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = getOrgIdFromSession(
      ctx.context.session as Record<string, unknown>
    );

    if (!orgId) {
      return EMPTY_ORG_USAGE;
    }

    const usage = await runWithSentryReport(
      apiEffect<RawOrgUsageData>("/v1/usage/current", {
        params: { org_id: orgId },
      })
    );

    return normalizeOrgUsageData(usage);
  });

/** Query options for the organization's current usage, quotas, and alerts. Refetches every 5 minutes. */
export const orgUsageQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.orgUsage.queryKey,
    queryFn: () => getOrgUsageServerFn(),
    refetchInterval: 300_000,
    refetchIntervalInBackground: false,
  });

/** Returns alerts where the organization is approaching a usage limit. */
export const useApproachingLimits = () => {
  const { data } = useQuery(orgUsageQueryOptions());
  if (!data?.alerts) {
    return [];
  }
  return data.alerts.filter((a) => a.type === "approaching_limit");
};
