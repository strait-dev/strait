import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import type { ListParams } from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import {
  deleteJobFn,
  fetchJob,
  fetchJobs,
  triggerJobFn,
  updateJobFn,
} from "@/lib/api";

type ListJobsInput = ListParams & { status?: string; search?: string };

export const jobsQueryOptions = (search?: ListJobsInput) =>
  queryOptions({
    queryKey: queryKeys.jobs.list(search).queryKey,
    queryFn: () => fetchJobs({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const jobQueryOptions = (id: string) =>
  queryOptions({
    queryKey: queryKeys.jobs.detail(id).queryKey,
    queryFn: () => fetchJob({ data: { id } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const useTriggerJob = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["jobs", "trigger"],
    mutationFn: (data: { id: string; payload?: unknown }) =>
      triggerJobFn({ data }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
    },
  });
};

export const usePauseJob = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["jobs", "pause"],
    mutationFn: (data: { id: string }) =>
      updateJobFn({ data: { id: data.id, enabled: false } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
    },
  });
};

export const useResumeJob = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["jobs", "resume"],
    mutationFn: (data: { id: string }) =>
      updateJobFn({ data: { id: data.id, enabled: true } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
    },
  });
};

export const useDeleteJob = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["jobs", "delete"],
    mutationFn: (data: { id: string }) => deleteJobFn({ data }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
    },
  });
};
