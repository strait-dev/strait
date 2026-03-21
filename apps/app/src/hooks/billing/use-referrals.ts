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
import { getOrgIdFromSession } from "./session";

export type Referral = {
  id: string;
  code: string;
  status: string;
  credit_microusd: number;
  referred_email: string;
  activated_at: string | null;
  created_at: string;
};

export type ReferralsResponse = {
  code: string;
  referrals: Referral[];
  total_credit_microusd: number;
};

const getReferralsServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = getOrgIdFromSession(
      ctx.context.session as Record<string, unknown>
    );

    if (!orgId) {
      return null;
    }

    return await runWithFallback(
      apiEffect<ReferralsResponse>("/v1/referrals", {
        params: { org_id: orgId },
      }),
      null
    );
  });

const createReferralCodeServerFn = createServerFn({ method: "POST" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = getOrgIdFromSession(
      ctx.context.session as Record<string, unknown>
    );

    if (!orgId) {
      throw new Error("No active organization");
    }

    return await runWithSentryReport(
      apiEffect<{ code: string }>("/v1/referrals", {
        method: "POST",
        body: { org_id: orgId },
      })
    );
  });

type ActivateInput = {
  code: string;
};

const activateReferralServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: ActivateInput) =>
    z.object({ code: z.string().min(1).max(100) }).parse(data)
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
      apiEffect<{ success: boolean }>("/v1/referrals/activate", {
        method: "POST",
        body: { org_id: orgId, code: data.code },
      })
    );
  });

export const referralsQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.billing.referrals.queryKey,
    queryFn: () => getReferralsServerFn(),
    refetchInterval: 300_000,
  });

export function useCreateReferralCode() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => createReferralCodeServerFn(),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.billing.referrals.queryKey,
      });
    },
  });
}

export function useActivateReferral() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (code: string) => activateReferralServerFn({ data: { code } }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.billing.referrals.queryKey,
      });
    },
  });
}
