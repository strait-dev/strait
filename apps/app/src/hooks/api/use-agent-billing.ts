import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

export interface AgentUsageResponse {
  agent_plan_tier: string;
  included_credit_usd: number;
  overage_usd: number;
  run_count: number;
  total_cost_microusd: number;
  total_tokens: number;
  total_tool_calls: number;
  upgrade_reason?: string;
  upgrade_recommended: boolean;
  used_credit_usd: number;
}

export interface AgentSpendingLimitResponse {
  enabled: boolean;
  limit_microusd: number;
  limit_usd: number;
}

export const fetchAgentUsage = createServerFn({ method: "GET" })
  .inputValidator((data: { orgId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<AgentUsageResponse> => {
    return await runWithSentryReport(
      apiEffect<AgentUsageResponse>(
        `/v1/agents/billing/usage?org_id=${data.orgId}`
      )
    );
  });

export const fetchAgentSpendingLimit = createServerFn({ method: "GET" })
  .inputValidator((data: { orgId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<AgentSpendingLimitResponse> => {
    return await runWithSentryReport(
      apiEffect<AgentSpendingLimitResponse>(
        `/v1/agents/billing/spending-limit?org_id=${data.orgId}`
      )
    );
  });

export const updateAgentSpendingLimit = createServerFn({ method: "POST" })
  .inputValidator((data: { orgId: string; limitMicrousd: number }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<AgentSpendingLimitResponse> => {
    return await runWithSentryReport(
      apiEffect<AgentSpendingLimitResponse>(
        `/v1/agents/billing/spending-limit?org_id=${data.orgId}`,
        {
          method: "PUT",
          body: { limit_microusd: data.limitMicrousd },
        }
      )
    );
  });

export const agentUsageQueryOptions = (orgId: string) =>
  queryOptions({
    queryKey: ["agent-billing-usage", orgId],
    queryFn: () => fetchAgentUsage({ data: { orgId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    enabled: !!orgId,
  });

export const agentSpendingLimitQueryOptions = (orgId: string) =>
  queryOptions({
    queryKey: ["agent-billing-spending-limit", orgId],
    queryFn: () => fetchAgentSpendingLimit({ data: { orgId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    enabled: !!orgId,
  });

export function useUpdateAgentSpendingLimit(orgId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (limitMicrousd: number) =>
      updateAgentSpendingLimit({ data: { orgId, limitMicrousd } }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["agent-billing-spending-limit", orgId],
      });
    },
  });
}
