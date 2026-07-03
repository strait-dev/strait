import { toast } from "@strait/ui/components/toast";
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
import { apiPath } from "@/lib/api-client.server";
import {
  apiEffect,
  runWithFallback,
  runWithSentryReport,
} from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import {
  requireActiveProjectAccess,
  requireActiveProjectAdmin,
} from "@/middlewares/require-access";

export type CreateWorkflowInput = {
  name: string;
  slug?: string;
  description?: string;
  enabled?: boolean;
  job_id: string;
};

function slugFromName(name: string) {
  const base = name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 48);
  const suffix = Math.random().toString(36).slice(2, 8);
  return `${base || "workflow"}-${suffix}`;
}

const emptyWorkflowStepsResponse: PaginatedResponse<WorkflowStep> = {
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

const fetchWorkflows = createServerFn({ method: "GET" })
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

const fetchWorkflow = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<Workflow> => {
    await requireActiveProjectAccess(context);
    return await runWithSentryReport(
      apiEffect<Workflow>(apiPath`/v1/workflows/${data.id}`)
    );
  });

const fetchWorkflowSteps = createServerFn({ method: "GET" })
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

const fetchWorkflowRuns = createServerFn({ method: "GET" })
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

const triggerWorkflowFn = createServerFn({ method: "POST" })
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

const createWorkflowFn = createServerFn({ method: "POST" })
  .inputValidator((data: CreateWorkflowInput) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<Workflow> => {
    const projectId = await requireActiveProjectAdmin(context);
    const response = await runWithSentryReport(
      apiEffect<Workflow & { steps?: WorkflowStep[] }>("/v1/workflows", {
        method: "POST",
        body: {
          project_id: projectId,
          name: data.name,
          slug: data.slug || slugFromName(data.name),
          description: data.description,
          enabled: data.enabled,
          steps: [
            {
              job_id: data.job_id,
              step_ref: "step-1",
              step_type: "job",
            },
          ],
        },
      })
    );
    return response;
  });

const updateWorkflowFn = createServerFn({ method: "POST" })
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

const deleteWorkflowFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<void> => {
    await requireActiveProjectAdmin(context);
    return await runWithSentryReport(
      apiEffect<void>(apiPath`/v1/workflows/${data.id}`, {
        method: "DELETE",
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
      toast.error("Failed to trigger workflow.");
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

export const useCreateWorkflow = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["workflows", "create"],
    mutationFn: async (data: CreateWorkflowInput) =>
      (await createWorkflowFn({ data })) as Workflow,
    onSuccess: (workflow) => {
      getPostHog()?.capture("workflow_created", {
        workflow_id: workflow.id,
      });
    },
    onError: (err) => {
      getPostHog()?.capture("mutation_error", {
        action: "workflow_created",
        error_message: err instanceof Error ? err.message : "Unknown error",
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.workflows._def });
    },
  });
};

export const useDeleteWorkflow = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["workflows", "delete"],
    mutationFn: (id: string) => deleteWorkflowFn({ data: { id } }),
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.workflows._def });
      const previousLists = queryClient.getQueriesData<
        PaginatedResponse<Workflow>
      >({ queryKey: queryKeys.workflows.list._def });

      queryClient.setQueriesData<PaginatedResponse<Workflow>>(
        { queryKey: queryKeys.workflows.list._def },
        (old) =>
          old
            ? {
                ...old,
                data: old.data.filter((workflow) => workflow.id !== id),
              }
            : old
      );

      return { previousLists };
    },
    onSuccess: (_data, id) => {
      getPostHog()?.capture("workflow_deleted", { workflow_id: id });
    },
    onError: (err, variables, context) => {
      toast.error("Failed to delete workflow.");
      if (context?.previousLists) {
        for (const [key, data] of context.previousLists) {
          queryClient.setQueryData(key, data);
        }
      }
      getPostHog()?.capture("mutation_error", {
        action: "workflow_deleted",
        error_message: err instanceof Error ? err.message : "Unknown error",
        workflow_id: variables,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.workflows._def });
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
      toast.error("Failed to pause workflow.");
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
      toast.error("Failed to resume workflow.");
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
