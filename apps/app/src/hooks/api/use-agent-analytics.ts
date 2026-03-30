import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

export type AgentTimelinePoint = {
  avg_duration_ms: number;
  bucket: string;
  completed: number;
  failed: number;
  total: number;
};

export type AgentRankingRow = {
  agent_id: string;
  agent_slug: string;
  avg_duration_ms: number;
  cost_microusd: number;
  runs: number;
  total_tokens: number;
};

export type AgentAnalyticsCostSummary = {
  avg_cost_microusd: number;
  checkpoint_count: number;
  completion_tokens: number;
  prompt_tokens: number;
  tool_call_count: number;
  total_cost_microusd: number;
  total_runs: number;
  total_tokens: number;
};

export const fetchAgentTimeline = createServerFn({ method: "GET" })
  .inputValidator((data: { from: string; to: string; bucket?: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<AgentTimelinePoint[]> => {
    try {
      return await runWithSentryReport(
        apiEffect<AgentTimelinePoint[]>(
          `/v1/analytics/agents/timeline?from=${data.from}&to=${data.to}&bucket=${data.bucket ?? "day"}`
        )
      );
    } catch {
      return [];
    }
  });

export const fetchAgentTopAgents = createServerFn({ method: "GET" })
  .inputValidator((data: { from: string; to: string; limit?: number }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<AgentRankingRow[]> => {
    try {
      return await runWithSentryReport(
        apiEffect<AgentRankingRow[]>(
          `/v1/analytics/agents/top?from=${data.from}&to=${data.to}&limit=${data.limit ?? 10}`
        )
      );
    } catch {
      return [];
    }
  });

export const fetchAgentAnalyticsCosts = createServerFn({ method: "GET" })
  .inputValidator((data: { from: string; to: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<AgentAnalyticsCostSummary | null> => {
    try {
      return await runWithSentryReport(
        apiEffect<AgentAnalyticsCostSummary>(
          `/v1/analytics/agents/costs?from=${data.from}&to=${data.to}`
        )
      );
    } catch {
      return null;
    }
  });

function dateRange(days: number) {
  const now = new Date();
  return {
    from: new Date(now.getTime() - days * 86_400_000).toISOString(),
    to: now.toISOString(),
  };
}

export const agentTimelineQueryOptions = (days = 30) => {
  const range = dateRange(days);
  return queryOptions({
    queryKey: [...queryKeys.agents._def, "timeline", days],
    queryFn: () => fetchAgentTimeline({ data: { ...range, bucket: "day" } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });
};

export const agentTopAgentsQueryOptions = (days = 30) => {
  const range = dateRange(days);
  return queryOptions({
    queryKey: [...queryKeys.agents._def, "top", days],
    queryFn: () => fetchAgentTopAgents({ data: { ...range, limit: 10 } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });
};

export const agentAnalyticsCostsQueryOptions = (days = 30) => {
  const range = dateRange(days);
  return queryOptions({
    queryKey: [...queryKeys.agents._def, "analytics-costs", days],
    queryFn: () => fetchAgentAnalyticsCosts({ data: range }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });
};
