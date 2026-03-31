import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import {
  type AgentListRow,
  buildAgentListRows,
} from "@/components/agents/agent-list-utils";
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

type CreateAgentInput = {
  projectId: string;
  name: string;
  slug: string;
  description?: string;
  model: string;
  config?: unknown;
};

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

export const fetchAgentListRows = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams) => data)
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<AgentListRow[]> => {
    const agents = await runWithSentryReport(
      apiEffect<Agent[]>("/v1/agents", {
        params: {
          cursor: data.cursor,
          limit: data.limit,
        },
      })
    );

    return buildAgentListRows(agents, {});
  });

export const createAgentFn = createServerFn({ method: "POST" })
  .inputValidator((data: CreateAgentInput) => data)
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<Agent> => {
    return await runWithSentryReport(
      apiEffect<Agent>("/v1/agents", {
        method: "POST",
        body: {
          project_id: data.projectId,
          name: data.name,
          slug: data.slug,
          description: data.description ?? "",
          model: data.model,
          config: data.config ?? {},
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

export const agentListRowsQueryOptions = (search?: ListParams) =>
  queryOptions({
    queryKey: queryKeys.agents.list(search).queryKey,
    queryFn: () => fetchAgentListRows({ data: search ?? {} }),
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

export const useCreateAgent = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationKey: ["agents", "create"],
    mutationFn: (data: CreateAgentInput) =>
      createAgentFn({ data }) as Promise<Agent>,
    onSuccess: (agent: Agent) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.agents._def });
      queryClient.setQueryData(
        queryKeys.agents.detail(agent.id).queryKey,
        agent
      );
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

export type AgentVersion = {
  agent_id: string;
  config_snapshot?: Record<string, object>;
  created_at: string;
  created_by?: string;
  deployed_at?: string;
  id: string;
  provider: string;
  provider_metadata?: Record<string, object>;
  status: string;
  updated_at: string;
  version: number;
};

const fetchAgentVersions = createServerFn({ method: "GET" })
  .inputValidator((data: { agentId: string; limit?: number }) => data)
  .middleware([authMiddleware])
  .handler(({ data }): Promise<AgentVersion[]> => {
    return runWithSentryReport(
      apiEffect<AgentVersion[]>(
        `/v1/agents/${data.agentId}/versions?limit=${data.limit ?? 20}`
      )
    );
  });

export const agentVersionsQueryOptions = (agentId: string) =>
  queryOptions({
    queryKey: [...queryKeys.agents._def, "versions", agentId],
    queryFn: () => fetchAgentVersions({ data: { agentId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export type AgentTopologyNode = {
  agent_id: string;
  agent_name: string;
  agent_slug: string;
};

export type AgentTopologyEdge = {
  message_count: number;
  source: string;
  target: string;
};

export type AgentTopology = {
  edges: AgentTopologyEdge[];
  nodes: AgentTopologyNode[];
};

const fetchAgentTopology = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler((): Promise<AgentTopology> => {
    return runWithSentryReport(apiEffect<AgentTopology>("/v1/agents/topology"));
  });

export const agentTopologyQueryOptions = () =>
  queryOptions({
    queryKey: [...queryKeys.agents._def, "topology"],
    queryFn: () => fetchAgentTopology(),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });
