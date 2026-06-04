/**
 * Project costs query hook.
 *
 * Fetches per-project cost breakdown for the current billing period from
 * `GET /v1/usage/projects`. Used by the project costs tab in the billing dashboard.
 */

import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveOrgAccess } from "@/middlewares/require-access";
import { type LimitAction, REFETCH_10M } from "./types";

/** Cost breakdown for a single project in the current billing period. */
export type ProjectCostEntry = {
  /** Project ID. */
  project_id: string;
  /** Project display name. */
  name: string;
  /** Total runs in the period. */
  runs: number;
  /** Run spend in micro-USD. */
  spend_microusd: number;
  /** Total cost in micro-USD. */
  total_microusd: number;
  /** Monthly budget in micro-USD, if set. */
  monthly_budget_microusd?: number;
  /** Budget action, if a budget is set. */
  budget_action?: LimitAction;
};

/** Server function to fetch per-project cost breakdown. */
const getProjectCostsServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = await requireActiveOrgAccess(ctx.context);

    const now = new Date();
    const fromDate = `${now.getUTCFullYear()}-${String(now.getUTCMonth() + 1).padStart(2, "0")}-01`;
    const toDate = `${now.getUTCFullYear()}-${String(now.getUTCMonth() + 1).padStart(2, "0")}-${String(now.getUTCDate()).padStart(2, "0")}`;

    return await runWithSentryReport(
      apiEffect<ProjectCostEntry[]>("/v1/usage/projects", {
        params: {
          org_id: orgId,
          from: fromDate,
          to: toDate,
        },
      })
    );
  });

/**
 * Query options for per-project cost breakdown in the current billing period.
 *
 * Refetches every 10 minutes.
 *
 * @returns TanStack Query options for `["billing", "projectCosts"]`.
 */
export const projectCostsQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.projectCosts.queryKey,
    queryFn: () => getProjectCostsServerFn(),
    refetchInterval: REFETCH_10M,
    refetchIntervalInBackground: false,
  });
