import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { queryKeys } from "@/hooks/query-keys";
import {
  apiEffect,
  runWithFallback,
  runWithSentryReport,
} from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireProjectAccess } from "@/middlewares/require-access";

export type ProjectBudgetData = {
  project_id: string;
  monthly_budget_microusd: number;
  budget_action: string;
  current_spend_microusd: number;
  percent_used: number;
};

type GetBudgetInput = {
  projectId: string;
};

const getProjectBudgetServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: GetBudgetInput) =>
    z.object({ projectId: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const activeOrgId = (context as Record<string, unknown>)
      .activeOrganizationId as string | undefined;
    await requireProjectAccess(context.user.id, data.projectId, activeOrgId);

    return await runWithFallback(
      apiEffect<ProjectBudgetData>("/v1/project-budget", {
        params: { project_id: data.projectId },
      }),
      null
    );
  });

export const projectBudgetQueryOptions = (projectId: string) =>
  queryOptions({
    queryKey: queryKeys.billing.projectBudget(projectId).queryKey,
    queryFn: () => getProjectBudgetServerFn({ data: { projectId } }),
    refetchInterval: 300_000,
  });

type SetBudgetInput = {
  projectId: string;
  budgetMicrousd: number;
  action: string;
};

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
    await requireProjectAccess(context.user.id, data.projectId, activeOrgId);

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

export function useSetProjectBudget() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (params: SetBudgetInput) =>
      setProjectBudgetServerFn({ data: params }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.billing.projectBudget(variables.projectId).queryKey,
      });
      queryClient.invalidateQueries({
        queryKey: queryKeys.billing.projectCosts.queryKey,
      });
    },
  });
}
