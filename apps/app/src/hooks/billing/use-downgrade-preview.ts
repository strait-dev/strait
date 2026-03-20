import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";

export type DowngradeImpact = {
  resource: string;
  current: number;
  limit: number;
  action: "ok" | "reduce" | "remove";
};

export type DowngradePreview = {
  current_plan: string;
  target_plan: string;
  impacts: DowngradeImpact[];
};

type DowngradePreviewInput = {
  targetTier: string;
};

const getDowngradePreviewServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: DowngradePreviewInput) => data)
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    const orgId = getOrgIdFromSession(
      context.session as Record<string, unknown>
    );

    if (!orgId) {
      return null;
    }

    return await runWithFallback(
      apiEffect<DowngradePreview>("/v1/downgrade-preview", {
        params: {
          org_id: orgId,
          target_tier: data.targetTier,
        },
      }),
      null
    );
  });

export const downgradePreviewQueryOptions = (targetTier: string) =>
  queryOptions({
    queryKey: queryKeys.billing.downgradePreview(targetTier).queryKey,
    queryFn: () => getDowngradePreviewServerFn({ data: { targetTier } }),
    enabled: !!targetTier,
  });
