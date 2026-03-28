import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import {
  type AgentCostSummary,
  buildAgentCostSummary,
} from "@/components/agents/agent-cost-utils";
import type {
  Agent,
  JobRun,
  PaginatedResponse,
  RunToolCall,
  RunUsage,
} from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

export const fetchAgentCostSummary = createServerFn({ method: "GET" })
  .inputValidator(
    (data: { agentId: string; runLimit?: number; usageLimit?: number }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<AgentCostSummary> => {
    const agent = await runWithSentryReport(
      apiEffect<Agent>(`/v1/agents/${data.agentId}`)
    );

    const runs = await runWithSentryReport(
      apiEffect<JobRun[]>(`/v1/agents/${data.agentId}/runs`, {
        params: {
          limit: data.runLimit ?? 50,
        },
      })
    );

    const usagePages = await Promise.all(
      runs.map((run) =>
        runWithSentryReport(
          apiEffect<PaginatedResponse<RunUsage>>(`/v1/runs/${run.id}/usage`, {
            params: {
              limit: data.usageLimit ?? 100,
            },
          })
        )
      )
    );

    const toolCallPages = await Promise.all(
      runs.map((run) =>
        runWithSentryReport(
          apiEffect<PaginatedResponse<RunToolCall>>(
            `/v1/runs/${run.id}/tool-calls`,
            {
              params: {
                limit: data.usageLimit ?? 100,
              },
            }
          )
        )
      )
    );

    const usageRecords = usagePages.flatMap((page) => page.data);
    const toolCalls = toolCallPages.flatMap((page) => page.data);
    return buildAgentCostSummary(runs, usageRecords, toolCalls, agent.config);
  });

export const agentCostSummaryQueryOptions = (agentId: string) =>
  queryOptions({
    queryKey: queryKeys.agents.costs(agentId).queryKey,
    queryFn: () =>
      fetchAgentCostSummary({
        data: {
          agentId,
        },
      }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });
