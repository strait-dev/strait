import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type {
  Agent,
  AgentDeployment,
  JobRun,
  ListParams,
} from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

export const fetchAgents = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<Agent[]> => {
    return await runWithSentryReport(
      apiEffect<Agent[]>("/v1/agents", {
        params: {
          limit: data.limit,
          cursor: data.cursor,
        },
      })
    );
  });

export const fetchAgent = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<Agent> => {
    return await runWithSentryReport(apiEffect<Agent>(`/v1/agents/${data.id}`));
  });

export const fetchAgentRuns = createServerFn({ method: "GET" })
  .inputValidator(
    (data: { agentId: string; limit?: number; offset?: number }) => data
  )
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<JobRun[]> => {
    return await runWithSentryReport(
      apiEffect<JobRun[]>(`/v1/agents/${data.agentId}/runs`, {
        params: {
          limit: data.limit,
          offset: data.offset,
        },
      })
    );
  });

export const deployAgentFn = createServerFn({ method: "POST" })
  .inputValidator((data: { agentId: string }) => data)
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<AgentDeployment> => {
    return await runWithSentryReport(
      apiEffect<AgentDeployment>(`/v1/agents/${data.agentId}/deploy`, {
        method: "POST",
      })
    );
  });

export const runAgentFn = createServerFn({ method: "POST" })
  .inputValidator((data: { agentId: string; payload?: unknown }) => data)
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<JobRun> => {
    return await runWithSentryReport(
      apiEffect<JobRun>(`/v1/agents/${data.agentId}/run`, {
        method: "POST",
        body: {
          payload: data.payload,
        },
      })
    );
  });

export const agentsQueryOptions = (search?: ListParams) =>
  queryOptions({
    queryKey: queryKeys.agents.list(search).queryKey,
    queryFn: () => fetchAgents({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const agentQueryOptions = (id: string) =>
  queryOptions({
    queryKey: queryKeys.agents.detail(id).queryKey,
    queryFn: () => fetchAgent({ data: { id } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const agentRunsQueryOptions = (
  agentId: string,
  search?: Pick<ListParams, "limit"> & {
    offset?: number;
  }
) =>
  queryOptions({
    queryKey: queryKeys.agents.runs(agentId, search).queryKey,
    queryFn: () =>
      fetchAgentRuns({
        data: {
          agentId,
          limit: search?.limit,
          offset: search?.offset,
        },
      }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const useDeployAgent = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationKey: ["agents", "deploy"],
    mutationFn: (data: { agentId: string }) => deployAgentFn({ data }),
    onSettled: (_result, _error, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.agents._def });
      queryClient.invalidateQueries({
        queryKey: queryKeys.agents.detail(variables.agentId).queryKey,
      });
    },
  });
};

export const useRunAgent = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationKey: ["agents", "run"],
    mutationFn: (data: { agentId: string; payload?: unknown }) =>
      runAgentFn({ data }),
    onSettled: (_result, _error, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.agents._def });
      queryClient.invalidateQueries({
        queryKey: queryKeys.agents.runs(variables.agentId).queryKey,
      });
      queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
    },
  });
};
