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
    const from = new Date(now.getFullYear(), now.getMonth(), 1);

    return await runWithSentryReport(
      apiEffect<ProjectCostEntry[]>("/v1/usage/projects", {
        params: {
          org_id: orgId,
          from: from.toISOString().split("T")[0],
          to: now.toISOString().split("T")[0],
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
