import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";

/** Cost breakdown for a single project in the current billing period. */
export type ProjectCostEntry = {
  project_id: string;
  name: string;
  runs: number;
  compute_microusd: number;
  ai_microusd: number;
  total_microusd: number;
  monthly_budget_microusd?: number;
  budget_action?: string;
};

const getProjectCostsServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = getOrgIdFromSession(
      ctx.context.session as Record<string, unknown>
    );

    if (!orgId) {
      return [] as ProjectCostEntry[];
    }

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

/** Query options for per-project cost breakdown in the current billing period. Refetches every 5 minutes. */
export const projectCostsQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.projectCosts.queryKey,
    queryFn: () => getProjectCostsServerFn(),
    refetchInterval: 300_000,
  });
