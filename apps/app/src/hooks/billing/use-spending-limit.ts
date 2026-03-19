import { useQuery } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

export type SpendingLimitData = {
  org_id: string;
  plan_tier: string;
  spending_limit_usd: number;
  limit_action: string;
  current_spend_usd: number;
  included_credit_usd: number;
  overage_spend_usd: number;
  is_hard_capped: boolean;
};

const getSpendingLimitServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = (ctx.context.session as Record<string, unknown>)
      .activeOrganizationId;

    if (!orgId || typeof orgId !== "string") {
      return null;
    }

    return await runWithFallback(
      apiEffect<SpendingLimitData>("/v1/spending-limit", {
        params: { org_id: orgId },
      }),
      null
    );
  });

export function useSpendingLimit() {
  return useQuery({
    queryKey: ["spending-limit"],
    queryFn: () => getSpendingLimitServerFn(),
    refetchInterval: 60_000,
  });
}
