import { queryOptions, useQuery } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import {
  EMPTY_ORG_USAGE,
  normalizeOrgUsageData,
  type RawOrgUsageData,
} from "@/hooks/billing/org-usage";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

const getOrgUsageServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = (ctx.context.session as Record<string, unknown>)
      .activeOrganizationId;

    if (!orgId || typeof orgId !== "string") {
      return EMPTY_ORG_USAGE;
    }

    const usage = await runWithFallback(
      apiEffect<RawOrgUsageData>("/v1/usage/current", {
        params: { org_id: orgId },
      }),
      EMPTY_ORG_USAGE
    );

    return normalizeOrgUsageData(usage);
  });

/** Query options for the organization's current usage, quotas, and alerts. Refetches every 60s. */
export const orgUsageQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.orgUsage.queryKey,
    queryFn: () => getOrgUsageServerFn(),
    refetchInterval: 60_000,
  });

/** Returns alerts where the organization is approaching a usage limit. */
export const useApproachingLimits = () => {
  const { data } = useQuery(orgUsageQueryOptions());
  if (!data?.alerts) {
    return [];
  }
  return data.alerts.filter((a) => a.type === "approaching_limit");
};
