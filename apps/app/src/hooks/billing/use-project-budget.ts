/**
 * Project budget query and mutation hooks.
 *
 * Fetches and updates per-project monthly budgets from
 * `GET/PUT /v1/project-budget`. Project budgets allow teams to set
 * cost limits per project independent of the org-level spending limit.
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
  requireProjectAccess,
  requireProjectAdmin,
} from "@/middlewares/require-access";
import { type LimitAction, REFETCH_10M } from "./types";

/** Project budget data from the backend. */
export type ProjectBudgetData = {
  /** Project ID. */
  project_id: string;
  /** Monthly budget in micro-USD. `-1` means no budget set. */
  monthly_budget_microusd: number;
  /** Action taken when the budget is reached. */
  budget_action: LimitAction;
  /** Current period spend in micro-USD. */
  current_spend_microusd: number;
  /** Percentage of budget used. */
  percent_used: number;
};

/** Input for the get budget server function. */
type GetBudgetInput = {
  projectId: string;
};

/** Server function to fetch a project's budget. */
const getProjectBudgetServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: GetBudgetInput) =>
    z.object({ projectId: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const activeOrgId = (context as Record<string, unknown>)
      .activeOrganizationId as string | undefined;
    await requireProjectAccess(context.user.id, data.projectId, activeOrgId);

    return await runWithSentryReport(
      apiEffect<ProjectBudgetData | null>("/v1/project-budget", {
        params: { project_id: data.projectId },
      })
    );
  });

/**
 * Query options for a project's monthly budget and current spend.
 *
 * Only enabled when a project ID is provided. Refetches every 10 minutes.
 *
 * @param projectId - The project to fetch the budget for.
 * @returns TanStack Query options for `["billing", "projectBudget", projectId]`.
 */
export const projectBudgetQueryOptions = (projectId: string) =>
  queryOptions({
    queryKey: queryKeys.billing.projectBudget(projectId).queryKey,
    queryFn: () => getProjectBudgetServerFn({ data: { projectId } }),
    enabled: !!projectId,
    refetchInterval: REFETCH_10M,
    refetchIntervalInBackground: false,
  });

/** Input for the set budget mutation. */
type SetBudgetInput = {
  projectId: string;
  budgetMicrousd: number;
  action: string;
};

/** Server function to update a project's budget. */
const setProjectBudgetServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: SetBudgetInput) =>
    z
      .object({
        projectId: z.string().min(1),
        budgetMicrousd: z.number(),
        action: z.string(),
      })
      .parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const activeOrgId = (context as Record<string, unknown>)
      .activeOrganizationId as string | undefined;
    await requireProjectAdmin(context.user.id, data.projectId, activeOrgId);

    return await runWithSentryReport(
      apiEffect<{ status: string }>("/v1/project-budget", {
        method: "PUT",
        body: {
          project_id: data.projectId,
          budget_microusd: data.budgetMicrousd,
          action: data.action,
        },
      })
    );
  });

/**
 * Mutation hook for setting a project's monthly budget.
 *
 * Invalidates both the project budget and project costs queries on settlement.
 *
 * @returns A TanStack Query mutation for project budget updates.
 */
export const useSetProjectBudget = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (params: SetBudgetInput) =>
      setProjectBudgetServerFn({ data: params }),
    onSettled: (_data, _err, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.billing.projectBudget(variables.projectId).queryKey,
      });
      queryClient.invalidateQueries({
        queryKey: queryKeys.billing.projectCosts.queryKey,
      });
    },
  });
};
