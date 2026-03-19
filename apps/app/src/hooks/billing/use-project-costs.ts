import { useQuery } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { authMiddleware } from "@/middlewares/auth";

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
    const orgId = (ctx.context.session as Record<string, unknown>)
      .activeOrganizationId;

    if (!orgId || typeof orgId !== "string") {
      return [] as ProjectCostEntry[];
    }

    const now = new Date();
    const from = new Date(now.getFullYear(), now.getMonth(), 1);

    try {
      const { apiRequest } = await import("@/lib/api-client.server");
      return await apiRequest<ProjectCostEntry[]>("/v1/usage/projects", {
        params: {
          org_id: orgId,
          from: from.toISOString().split("T")[0],
          to: now.toISOString().split("T")[0],
        },
      });
    } catch {
      return [] as ProjectCostEntry[];
    }
  });

export function useProjectCosts() {
  return useQuery({
    queryKey: ["project-costs"],
    queryFn: () => getProjectCostsServerFn(),
    refetchInterval: 300_000,
  });
}
