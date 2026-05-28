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
  WorkflowRunChainEntry,
  WorkflowStep,
  WorkflowStepRun,
} from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getPostHog } from "@/lib/analytics";
import { apiPath } from "@/lib/api-client.server";
import {
  apiEffect,
  runWithFallback,
  runWithSentryReport,
} from "@/lib/effect-api.server";
import type { ContinueVersionStrategy } from "@/lib/workflow-continue";
import { authMiddleware } from "@/middlewares/auth";
import {
  requireActiveProjectAccess,
  requireActiveProjectAdmin,
} from "@/middlewares/require-access";

const emptyWorkflowStepsResponse: PaginatedResponse<WorkflowStep> = {
  data: [],
  has_more: false,
};
const emptyWorkflowStepRunsResponse: PaginatedResponse<WorkflowStepRun> = {
  data: [],
  has_more: false,
};
const emptyWorkflowVersionsResponse: PaginatedResponse<WorkflowVersionSummary> =
  {
    data: [],
    has_more: false,
  };

type WorkflowVersionSummary = {
  version_id?: string;
};

/** Normalize Strait API list responses, which may be raw arrays on older handlers. */
function dataFromPaginatedOrArray<T>(response: PaginatedResponse<T> | T[]) {
  return Array.isArray(response) ? response : response.data;
}

export const fetchWorkflows = createServerFn({ method: "GET" })
  .inputValidator(
    (data: { limit?: number; cursor?: string; search?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<PaginatedResponse<Workflow>> => {
    await requireActiveProjectAccess(context);
    return await runWithSentryReport(
      apiEffect<PaginatedResponse<Workflow>>("/v1/workflows", {
        params: {
          limit: data.limit,
          cursor: data.cursor,
          search: data.search,
        },
      })
    );
  });

export const fetchWorkflow = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<Workflow> => {
    await requireActiveProjectAccess(context);
    return await runWithSentryReport(
      apiEffect<Workflow>(apiPath`/v1/workflows/${data.id}`)
    );
  });

export const fetchWorkflowSteps = createServerFn({ method: "GET" })
  .inputValidator((data: { workflowId: string }) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<WorkflowStep[]> => {
      await requireActiveProjectAccess(context);
      const resp = await runWithFallback(
        apiEffect<PaginatedResponse<WorkflowVersionSummary>>(
          apiPath`/v1/workflows/${data.workflowId}/versions`,
          { params: { limit: 1 } }
        ),
        emptyWorkflowVersionsResponse
      );
      const versions = dataFromPaginatedOrArray(resp);
      if (versions.length > 0) {
        const latestVersion = versions[0];
        if (!latestVersion.version_id) {
          return [] as WorkflowStep[];
        }
        const stepsResp = await runWithFallback(
          apiEffect<PaginatedResponse<WorkflowStep>>(
            apiPath`/v1/workflows/${data.workflowId}/versions/${latestVersion.version_id}/steps`
          ),
          emptyWorkflowStepsResponse
        );
        return dataFromPaginatedOrArray(stepsResp);
      }
      return [] as WorkflowStep[];
    }
  );

export const fetchWorkflowRuns = createServerFn({ method: "GET" })
  .inputValidator(
    (data: { workflowId: string; limit?: number; cursor?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<PaginatedResponse<WorkflowRun>> => {
      await requireActiveProjectAccess(context);
      return await runWithSentryReport(
        apiEffect<PaginatedResponse<WorkflowRun>>(
          apiPath`/v1/workflows/${data.workflowId}/runs`,
          { params: { limit: data.limit, cursor: data.cursor } }
        )
      );
    }
  );

export const fetchWorkflowRun = createServerFn({ method: "GET" })
  .inputValidator((data: { workflowRunId: string }) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<WorkflowRun> => {
      await requireActiveProjectAccess(context);
      return await runWithSentryReport(
        apiEffect<WorkflowRun>(apiPath`/v1/workflow-runs/${data.workflowRunId}`)
      );
    }
  );

export const fetchWorkflowRunSteps = createServerFn({ method: "GET" })
  .inputValidator((data: { workflowRunId: string }) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<WorkflowStepRun[]> => {
      await requireActiveProjectAccess(context);
      const resp = await runWithFallback(
        apiEffect<PaginatedResponse<WorkflowStepRun>>(
          apiPath`/v1/workflow-runs/${data.workflowRunId}/steps`
        ),
        emptyWorkflowStepRunsResponse
      );
      return dataFromPaginatedOrArray(resp);
    }
  );

export const triggerWorkflowFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: {
      workflowId: string;
      payload?: unknown;
      tags?: Record<string, string>;
    }) => data
  )
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<WorkflowRun> => {
      await requireActiveProjectAdmin(context);
      return await runWithSentryReport(
        apiEffect<WorkflowRun>(
          apiPath`/v1/workflows/${data.workflowId}/trigger`,
          {
            method: "POST",
            body: { payload: data.payload, tags: data.tags },
          }
        )
      );
    }
  );

export const continueWorkflowRunAsNewFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: {
      workflowRunId: string;
      input?: unknown;
      versionStrategy?: ContinueVersionStrategy;
    }) => data
  )
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<WorkflowRun> => {
      await requireActiveProjectAdmin(context);
      return await runWithSentryReport(
        apiEffect<WorkflowRun>(
          apiPath`/v1/workflow-runs/${data.workflowRunId}/continue-as-new`,
          {
            method: "POST",
            body: {
              input: data.input,
              versionStrategy: data.versionStrategy,
            },
          }
        )
      );
    }
  );

export const fetchWorkflowRunChain = createServerFn({ method: "GET" })
  .inputValidator(
    (data: { workflowRunId: string; limit?: number; cursor?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(
    async ({
      context,
      data,
    }): Promise<PaginatedResponse<WorkflowRunChainEntry>> => {
      await requireActiveProjectAccess(context);
      return await runWithSentryReport(
        apiEffect<PaginatedResponse<WorkflowRunChainEntry>>(
          apiPath`/v1/workflow-runs/${data.workflowRunId}/chain`,
          { params: { limit: data.limit, cursor: data.cursor } }
        )
      );
    }
  );

export const updateWorkflowFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string; enabled?: boolean }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<Workflow> => {
    await requireActiveProjectAdmin(context);
    const { id, ...body } = data;
    return await runWithSentryReport(
      apiEffect<Workflow>(apiPath`/v1/workflows/${id}`, {
        method: "PATCH",
        body,
      })
    );
  });

type ListWorkflowsInput = { limit?: number; cursor?: string; search?: string };

export const workflowsQueryOptions = (search?: ListWorkflowsInput | string) => {
  const params: ListWorkflowsInput =
    typeof search === "string" ? { search } : (search ?? {});
  return queryOptions({
    queryKey: queryKeys.workflows.list(params).queryKey,
    queryFn: () => fetchWorkflows({ data: params }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });
};

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

export const workflowRunQueryOptions = (workflowRunId: string) =>
  queryOptions({
    queryKey: queryKeys.workflows.run(workflowRunId).queryKey,
    queryFn: () => fetchWorkflowRun({ data: { workflowRunId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const workflowRunStepsQueryOptions = (workflowRunId: string) =>
  queryOptions({
    queryKey: queryKeys.workflows.runSteps(workflowRunId).queryKey,
    queryFn: () => fetchWorkflowRunSteps({ data: { workflowRunId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const workflowRunChainQueryOptions = (workflowRunId: string) =>
  queryOptions({
    queryKey: queryKeys.workflows.chain(workflowRunId).queryKey,
    queryFn: () => fetchWorkflowRunChain({ data: { workflowRunId } }),
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

export const useContinueWorkflowRunAsNew = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["workflows", "continue-as-new"],
    mutationFn: (params: {
      workflowRunId: string;
      workflowId: string;
      input?: unknown;
      versionStrategy?: ContinueVersionStrategy;
    }): Promise<WorkflowRun> =>
      continueWorkflowRunAsNewFn({
        data: {
          workflowRunId: params.workflowRunId,
          input: params.input,
          versionStrategy: params.versionStrategy,
        },
      }) as Promise<WorkflowRun>,
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("workflow_run_continued_as_new", {
        workflow_id: variables.workflowId,
        workflow_run_id: variables.workflowRunId,
        version_strategy: variables.versionStrategy ?? "repin",
      });
    },
    onError: (err, variables) => {
      getPostHog()?.capture("mutation_error", {
        action: "workflow_run_continued_as_new",
        error_message: err instanceof Error ? err.message : "Unknown error",
        workflow_run_id: variables.workflowRunId,
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
