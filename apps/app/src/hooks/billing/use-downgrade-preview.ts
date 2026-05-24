/**
 * Downgrade preview query hook.
 *
 * Fetches a preview of the resource impacts when downgrading to a lower
 * plan tier from `GET /v1/downgrade-preview`.
 */

import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { queryKeys } from "@/hooks/query-keys";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveOrgAccess } from "@/middlewares/require-access";
import type { PlanTierSlug, ResourceAction } from "./types";

/** Impact on a single resource dimension when downgrading. */
export type DowngradeImpact = {
  /** Resource name (e.g. "projects", "members", "concurrent_runs"). */
  resource: string;
  /** Current value or count. */
  current: number;
  /** New limit after downgrade. */
  limit: number;
  /** Whether the resource is unaffected, reduced, or removed. */
  action: ResourceAction;
};

/** Full downgrade preview response from the backend. */
export type DowngradePreview = {
  /** The plan currently subscribed to. */
  current_plan: PlanTierSlug;
  /** The target plan to downgrade to. */
  target_plan: PlanTierSlug;
  /** All resource impacts. */
  impacts: DowngradeImpact[];
  /** Date when the downgrade takes effect (end of current period). */
  effective_date?: string;
  /** Resources requiring manual user action (e.g. removing projects). */
  manual_actions?: DowngradeImpact[];
  /** Resources that will be auto-disabled on downgrade. */
  auto_disabled?: DowngradeImpact[];
};

/** Input for the downgrade preview server function. */
type DowngradePreviewInput = {
  targetTier: string;
};

/** Server function to fetch the downgrade preview from the backend. */
const getDowngradePreviewServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: DowngradePreviewInput) =>
    z
      .object({
        targetTier: z.enum([
          "free",
          "starter",
          "pro",
          "scale",
          "business",
          "enterprise",
        ]),
      })
      .parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    const orgId = await requireActiveOrgAccess(context);

    return await runWithSentryReport(
      apiEffect<DowngradePreview>("/v1/downgrade-preview", {
        params: {
          org_id: orgId,
          target_tier: data.targetTier,
        },
      })
    );
  });

/**
 * Query options for the downgrade preview to a specific target tier.
 *
 * Only enabled when a target tier is provided. Not refetched on interval
 * since downgrade previews are point-in-time calculations.
 *
 * @param targetTier - The plan tier to preview downgrading to.
 * @returns TanStack Query options for `["billing", "downgradePreview", targetTier]`.
 */
export const downgradePreviewQueryOptions = (targetTier: string) =>
  queryOptions({
    queryKey: queryKeys.billing.downgradePreview(targetTier).queryKey,
    queryFn: () => getDowngradePreviewServerFn({ data: { targetTier } }),
    enabled: !!targetTier,
  });
