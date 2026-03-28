import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import {
  buildAgentCostSummary,
  type AgentCostSummary,
} from "@/components/agents/agent-cost-utils";
import type {
  JobRun,
  PaginatedResponse,
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

    const usageRecords = usagePages.flatMap((page) => page.data);
    return buildAgentCostSummary(runs, usageRecords);
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
