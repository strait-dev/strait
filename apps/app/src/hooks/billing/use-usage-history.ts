import { useQuery } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

export type UsageHistoryEntry = {
  date: string;
  runs_count: number;
  compute_cost_microusd: number;
  ai_tokens: number;
  ai_cost_microusd: number;
};

const getUsageHistoryServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = (ctx.context.session as Record<string, unknown>)
      .activeOrganizationId;

    if (!orgId || typeof orgId !== "string") {
      return [] as UsageHistoryEntry[];
    }

    const now = new Date();
    const from = new Date(now.getFullYear(), now.getMonth(), 1);
    const to = now;

    return await runWithFallback(
      apiEffect<UsageHistoryEntry[]>("/v1/usage/history", {
        params: {
          org_id: orgId,
          from: from.toISOString().split("T")[0],
          to: to.toISOString().split("T")[0],
        },
      }),
      [] as UsageHistoryEntry[]
    );
  });

export function useUsageHistory() {
  return useQuery({
    queryKey: ["usage-history"],
    queryFn: () => getUsageHistoryServerFn(),
    refetchInterval: 300_000,
  });
}
