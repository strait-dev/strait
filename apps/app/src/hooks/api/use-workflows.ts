import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type {
  PaginatedResponse,
  Workflow,
  WorkflowRun,
  WorkflowStep,
} from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getPostHog } from "@/lib/analytics";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";


export const fetchWorkflows = createServerFn({ method: "GET" })
  .inputValidator(
    (data: { limit?: number; cursor?: string; search?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<PaginatedResponse<Workflow>> => {
    return await runWithSentryReport(
      apiEffect<PaginatedResponse<Workflow>>("/v1/workflows", {
        params: { limit: data.limit, cursor: data.cursor, search: data.search },
      })
    );
  });

export const fetchWorkflow = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<Workflow> => {
    return await runWithSentryReport(
      apiEffect<Workflow>(`/v1/workflows/${data.id}`)
    );
  });

export const fetchWorkflowSteps = createServerFn({ method: "GET" })
  .inputValidator((data: { workflowId: string }) => data)
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<WorkflowStep[]> => {
    const resp = await runWithSentryReport(
      apiEffect<PaginatedResponse<WorkflowStep>>(
        `/v1/workflows/${data.workflowId}/versions`,
        { params: { limit: 1 } }
      )
    );
    if (resp.data.length > 0) {
      const latestVersion = resp.data[0] as unknown as { id: string };
      const stepsResp = await runWithSentryReport(
        apiEffect<PaginatedResponse<WorkflowStep>>(
          `/v1/workflows/${data.workflowId}/versions/${latestVersion.id}/steps`
        )
      );
      return stepsResp.data;
    }
    return [] as WorkflowStep[];
  });

export const fetchWorkflowRuns = createServerFn({ method: "GET" })
  .inputValidator(
    (data: { workflowId: string; limit?: number; cursor?: string }) => data
  )
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<PaginatedResponse<WorkflowRun>> => {
    return await runWithSentryReport(
      apiEffect<PaginatedResponse<WorkflowRun>>(
        `/v1/workflows/${data.workflowId}/runs`,
        { params: { limit: data.limit, cursor: data.cursor } }
      )
    );
  });

export const triggerWorkflowFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: {
      workflowId: string;
      payload?: unknown;
      tags?: Record<string, string>;
    }) => data
  )
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<WorkflowRun> => {
    return await runWithSentryReport(
      apiEffect<WorkflowRun>(`/v1/workflows/${data.workflowId}/trigger`, {
        method: "POST",
        body: { payload: data.payload, tags: data.tags },
      })
    );
  });

export const updateWorkflowFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string; enabled?: boolean }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<Workflow> => {
    const { id, ...body } = data;
    return await runWithSentryReport(
      apiEffect<Workflow>(`/v1/workflows/${id}`, {
        method: "PATCH",
        body,
      })
    );
  });


export const workflowsQueryOptions = (search?: string) =>
  queryOptions({
    queryKey: queryKeys.workflows.list({ search }).queryKey,
    queryFn: () => fetchWorkflows({ data: { search } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const workflowQueryOptions = (id: string) =>
  queryOptions({
    queryKey: queryKeys.workflows.detail(id).queryKey,
    queryFn: () => fetchWorkflow({ data: { id } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const workflowStepsQueryOptions = (workflowId: string) =>
  queryOptions({
    queryKey: queryKeys.workflows.steps(workflowId).queryKey,
    queryFn: () => fetchWorkflowSteps({ data: { workflowId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const workflowRunsQueryOptions = (workflowId: string) =>
  queryOptions({
    queryKey: queryKeys.workflows.runs(workflowId).queryKey,
    queryFn: () => fetchWorkflowRuns({ data: { workflowId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });


export const useTriggerWorkflow = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["workflows", "trigger"],
    mutationFn: (params: { workflowId: string; payload?: unknown }) =>
      triggerWorkflowFn({ data: params }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("workflow_triggered", {
        workflow_id: variables.workflowId,
      });
    },
    onError: (err, variables) => {
      getPostHog()?.capture("mutation_error", {
        action: "workflow_triggered",
        error_message: err instanceof Error ? err.message : "Unknown error",
        workflow_id: variables.workflowId,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.workflows._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
    },
  });
};

export const usePauseWorkflow = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["workflows", "pause"],
    mutationFn: (params: { workflowId: string }) =>
      updateWorkflowFn({ data: { id: params.workflowId, enabled: false } }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("workflow_paused", {
        workflow_id: variables.workflowId,
      });
    },
    onMutate: async (params) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.workflows._def });

      const previousDetail = queryClient.getQueryData<Workflow>(
        queryKeys.workflows.detail(params.workflowId).queryKey
      );

      queryClient.setQueryData<Workflow>(
        queryKeys.workflows.detail(params.workflowId).queryKey,
        (old) => (old ? { ...old, enabled: false } : old)
      );

      queryClient.setQueriesData<PaginatedResponse<Workflow>>(
        { queryKey: queryKeys.workflows.list._def },
        (old) =>
          old
            ? {
                ...old,
                data: old.data.map((wf) =>
                  wf.id === params.workflowId ? { ...wf, enabled: false } : wf
                ),
              }
            : old
      );

      return { previousDetail };
    },
    onError: (_err, params, context) => {
      if (context?.previousDetail) {
        queryClient.setQueryData(
          queryKeys.workflows.detail(params.workflowId).queryKey,
          context.previousDetail
        );
      }
      getPostHog()?.capture("mutation_error", {
        action: "workflow_paused",
        error_message: _err instanceof Error ? _err.message : "Unknown error",
        workflow_id: params.workflowId,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.workflows._def });
    },
  });
};

export const useResumeWorkflow = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["workflows", "resume"],
    mutationFn: (params: { workflowId: string }) =>
      updateWorkflowFn({ data: { id: params.workflowId, enabled: true } }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("workflow_resumed", {
        workflow_id: variables.workflowId,
      });
    },
    onMutate: async (params) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.workflows._def });

      const previousDetail = queryClient.getQueryData<Workflow>(
        queryKeys.workflows.detail(params.workflowId).queryKey
      );

      queryClient.setQueryData<Workflow>(
        queryKeys.workflows.detail(params.workflowId).queryKey,
        (old) => (old ? { ...old, enabled: true } : old)
      );

      queryClient.setQueriesData<PaginatedResponse<Workflow>>(
        { queryKey: queryKeys.workflows.list._def },
        (old) =>
          old
            ? {
                ...old,
                data: old.data.map((wf) =>
                  wf.id === params.workflowId ? { ...wf, enabled: true } : wf
                ),
              }
            : old
      );

      return { previousDetail };
    },
    onError: (_err, params, context) => {
      if (context?.previousDetail) {
        queryClient.setQueryData(
          queryKeys.workflows.detail(params.workflowId).queryKey,
          context.previousDetail
        );
      }
      getPostHog()?.capture("mutation_error", {
        action: "workflow_resumed",
        error_message: _err instanceof Error ? _err.message : "Unknown error",
        workflow_id: params.workflowId,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.workflows._def });
    },
  });
};
