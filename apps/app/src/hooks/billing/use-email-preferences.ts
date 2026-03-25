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
import { getOrgIdFromSession } from "./session";

export type EmailPreferences = {
  monthly_usage_email: boolean;
};

const getEmailPreferencesServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = getOrgIdFromSession(
      ctx.context.session as Record<string, unknown>
    );

    if (!orgId) {
      return { monthly_usage_email: true };
    }

    return await runWithSentryReport(
      apiEffect<EmailPreferences>("/v1/usage/email-preferences", {
        params: { org_id: orgId },
      })
    );
  });

export const emailPreferencesQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.emailPreferences.queryKey,
    queryFn: () => getEmailPreferencesServerFn(),
    refetchInterval: 300_000,
    refetchIntervalInBackground: false,
  });

type UpdateEmailPreferencesInput = {
  monthlyUsageEmail: boolean;
};

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
    const orgId = getOrgIdFromSession(
      context.session as Record<string, unknown>
    );

    if (!orgId) {
      throw new Error("No active organization");
    }

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
