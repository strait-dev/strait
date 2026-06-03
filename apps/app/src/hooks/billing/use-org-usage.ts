/**
 * Organization usage data fetching hooks.
 *
 * Fetches the current billing period's usage, quotas, alerts, and enterprise
 * contract details from the Go backend via `/v1/usage/current`.
 */

import { queryOptions, useQuery } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import {
  EMPTY_ORG_USAGE,
  normalizeOrgUsageData,
} from "@/hooks/billing/org-usage";
import { queryKeys } from "@/hooks/query-keys";
import {
  apiEffectWithSchema,
  runWithSentryReport,
} from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveOrgAccess } from "@/middlewares/require-access";
import { OrgUsageResponseSchema } from "./schemas";
import { REFETCH_5M } from "./types";

/**
 * Server function that fetches the current billing period's usage data.
 *
 * Returns {@link EMPTY_ORG_USAGE} when no active organization is found.
 * Normalizes the response shape before it reaches client components.
 */
const getOrgUsageServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = await requireActiveOrgAccess(ctx.context);

    const usage = await runWithSentryReport(
      apiEffectWithSchema("/v1/usage/current", OrgUsageResponseSchema, {
        params: { org_id: orgId },
      })
    );

    return normalizeOrgUsageData(usage);
  });

/**
 * Query options for the organization's current usage, quotas, and alerts.
 *
 * Refetches every 5 minutes to keep the billing dashboard up to date.
 *
 * @returns TanStack Query options for `["billing", "orgUsage"]`.
 */
export const orgUsageQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.orgUsage.queryKey,
    queryFn: () => getOrgUsageServerFn(),
    refetchInterval: REFETCH_5M,
    refetchIntervalInBackground: false,
  });

/**
 * Returns alerts where the organization is approaching a usage limit.
 *
 * Filters the full alert list to only `"approaching_limit"` type alerts,
 * which are shown as warning banners in the billing dashboard.
 *
 * @returns Array of approaching-limit alerts, or empty array if none.
 */
export const useApproachingLimits = () => {
  const { data } = useQuery(orgUsageQueryOptions());
  if (!data?.alerts) {
    return [];
  }
  return data.alerts.filter((a) => a.type === "approaching_limit");
};
