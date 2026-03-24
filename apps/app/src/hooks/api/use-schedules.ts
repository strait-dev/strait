import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import type { Job, ListParams, PaginatedResponse } from "@/hooks/api/types";
import { fetchJobs, triggerJobFn, updateJobFn } from "@/hooks/api/use-jobs";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";

/**
 * Schedules are cron-enabled jobs. We reuse the jobs endpoint and
 * filter server-side for jobs that have a cron expression set.
 */
export const schedulesQueryOptions = (search?: ListParams) =>
  queryOptions({
    queryKey: queryKeys.schedules.list(search).queryKey,
    queryFn: () => fetchJobs({ data: { ...search, status: "cron" } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const usePauseSchedule = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["schedules", "pause"],
    mutationFn: (data: { id: string }) =>
      updateJobFn({ data: { id: data.id, enabled: false } }),
    onMutate: async (data) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.schedules._def });

      queryClient.setQueriesData<PaginatedResponse<Job>>(
        { queryKey: queryKeys.schedules.list._def },
        (old) =>
          old
            ? {
                ...old,
                data: old.data.map((job) =>
                  job.id === data.id ? { ...job, enabled: false } : job
                ),
              }
            : old
      );
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
    },
  });
};

export const useResumeSchedule = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["schedules", "resume"],
    mutationFn: (data: { id: string }) =>
      updateJobFn({ data: { id: data.id, enabled: true } }),
    onMutate: async (data) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.schedules._def });

      queryClient.setQueriesData<PaginatedResponse<Job>>(
        { queryKey: queryKeys.schedules.list._def },
        (old) =>
          old
            ? {
                ...old,
                data: old.data.map((job) =>
                  job.id === data.id ? { ...job, enabled: true } : job
                ),
              }
            : old
      );
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
    },
  });
};

export const useTriggerSchedule = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["schedules", "trigger"],
    mutationFn: (data: { id: string }) => triggerJobFn({ data }),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
    },
  });
};
