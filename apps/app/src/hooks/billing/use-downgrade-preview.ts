import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
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
  effective_date?: string;
  manual_actions?: DowngradeImpact[];
  auto_disabled?: DowngradeImpact[];
};

type DowngradePreviewInput = {
  targetTier: string;
};

const getDowngradePreviewServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: DowngradePreviewInput) =>
    z
      .object({
        targetTier: z.enum(["free", "starter", "pro", "enterprise"]),
      })
      .parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    const orgId = getOrgIdFromSession(
      context.session as Record<string, unknown>
    );

    if (!orgId) {
      return null;
    }

    return await runWithSentryReport(
      apiEffect<DowngradePreview>("/v1/downgrade-preview", {
        params: {
          org_id: orgId,
          target_tier: data.targetTier,
        },
      })
    );
  });

export const downgradePreviewQueryOptions = (targetTier: string) =>
  queryOptions({
    queryKey: queryKeys.billing.downgradePreview(targetTier).queryKey,
    queryFn: () => getDowngradePreviewServerFn({ data: { targetTier } }),
    enabled: !!targetTier,
  });
