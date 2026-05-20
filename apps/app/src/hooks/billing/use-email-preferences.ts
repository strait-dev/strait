/**
 * Email preferences query and mutation hooks.
 *
 * Fetches and updates the organization's billing email preferences from
 * `GET/PUT /v1/usage/email-preferences`. Controls opt-in for monthly
 * usage report PDF emails.
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
  requireActiveOrgAccess,
  requireActiveOrgAdmin,
} from "@/middlewares/require-access";
import { REFETCH_5M } from "./types";

/** Organization email notification preferences. */
export type EmailPreferences = {
  /** Whether monthly usage report emails are enabled. */
  monthly_usage_email: boolean;
};

/** Server function to fetch email preferences. */
const getEmailPreferencesServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = await requireActiveOrgAccess(ctx.context);

    return await runWithSentryReport(
      apiEffect<EmailPreferences>("/v1/usage/email-preferences", {
        params: { org_id: orgId },
      })
    );
  });

/**
 * Query options for the organization's email notification preferences.
 *
 * Refetches every 5 minutes.
 *
 * @returns TanStack Query options for `["billing", "emailPreferences"]`.
 */
export const emailPreferencesQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.emailPreferences.queryKey,
    queryFn: () => getEmailPreferencesServerFn(),
    refetchInterval: REFETCH_5M,
    refetchIntervalInBackground: false,
  });

/** Input for the email preferences update mutation. */
type UpdateEmailPreferencesInput = {
  monthlyUsageEmail: boolean;
};

/** Server function to update email preferences. */
const updateEmailPreferencesServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: UpdateEmailPreferencesInput) =>
    z
      .object({
        monthlyUsageEmail: z.boolean(),
      })
      .parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    const orgId = await requireActiveOrgAdmin(context);

    return await runWithSentryReport(
      apiEffect<{ status: string }>("/v1/usage/email-preferences", {
        method: "PUT",
        params: { org_id: orgId },
        body: {
          monthly_usage_email: data.monthlyUsageEmail,
        },
      })
    );
  });

/**
 * Mutation hook for updating email notification preferences.
 *
 * Invalidates the email preferences query on settlement.
 *
 * @returns A TanStack Query mutation for email preference updates.
 */
export const useUpdateEmailPreferences = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (params: UpdateEmailPreferencesInput) =>
      updateEmailPreferencesServerFn({ data: params }),
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.billing.emailPreferences.queryKey,
      });
    },
  });
};
