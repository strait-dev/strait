import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import {
  fetchWorkflow,
  fetchWorkflowRuns,
  fetchWorkflowSteps,
  fetchWorkflows,
  triggerWorkflowFn,
  updateWorkflowFn,
} from "@/lib/api";

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
    onSuccess: () => {
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
    onSuccess: () => {
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.workflows._def });
    },
  });
};
