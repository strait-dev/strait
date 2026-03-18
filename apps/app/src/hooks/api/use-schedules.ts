import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import type { ListParams } from "@/hooks/api/types";
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
    onSuccess: () => {
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
    onSuccess: () => {
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
    },
  });
};
