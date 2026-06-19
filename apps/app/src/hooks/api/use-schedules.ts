import { toast } from "@strait/ui/components/toast";
import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import type { Job, ListParams, PaginatedResponse } from "@/hooks/api/types";
import {
  deleteJobFn,
  fetchJobs,
  pauseJobFn,
  resumeJobFn,
  triggerJobFn,
} from "@/hooks/api/use-jobs";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getPostHog } from "@/lib/analytics";

/**
 * Schedules are cron-enabled jobs. We reuse the jobs endpoint and
 * filter server-side for jobs that have a cron expression set.
 */
type ListSchedulesInput = ListParams & { search?: string };

export const schedulesQueryOptions = (search?: ListSchedulesInput) =>
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
    mutationFn: (data: { id: string }) => pauseJobFn({ data }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("schedule_paused", { schedule_id: variables.id });
    },
    onError: (err, variables) => {
      toast.error("Failed to pause schedule.");
      getPostHog()?.capture("mutation_error", {
        action: "schedule_paused",
        error_message: err instanceof Error ? err.message : "Unknown error",
        schedule_id: variables.id,
      });
    },
    onMutate: async (data) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.schedules._def });

      queryClient.setQueriesData<PaginatedResponse<Job>>(
        { queryKey: queryKeys.schedules.list._def },
        (old) =>
          old
            ? {
                ...old,
                data: old.data.map((job) =>
                  job.id === data.id ? { ...job, paused: true } : job
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
    mutationFn: (data: { id: string }) => resumeJobFn({ data }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("schedule_resumed", { schedule_id: variables.id });
    },
    onError: (err, variables) => {
      toast.error("Failed to resume schedule.");
      getPostHog()?.capture("mutation_error", {
        action: "schedule_resumed",
        error_message: err instanceof Error ? err.message : "Unknown error",
        schedule_id: variables.id,
      });
    },
    onMutate: async (data) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.schedules._def });

      queryClient.setQueriesData<PaginatedResponse<Job>>(
        { queryKey: queryKeys.schedules.list._def },
        (old) =>
          old
            ? {
                ...old,
                data: old.data.map((job) =>
                  job.id === data.id ? { ...job, paused: false } : job
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
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("schedule_triggered", {
        schedule_id: variables.id,
      });
    },
    onError: (err, variables) => {
      toast.error("Failed to trigger schedule.");
      getPostHog()?.capture("mutation_error", {
        action: "schedule_triggered",
        error_message: err instanceof Error ? err.message : "Unknown error",
        schedule_id: variables.id,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
    },
  });
};

export const useDeleteSchedule = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["schedules", "delete"],
    mutationFn: (id: string) => deleteJobFn({ data: { id } }),
    onSuccess: (_data, id) => {
      getPostHog()?.capture("schedule_deleted", { schedule_id: id });
    },
    onError: (err, id) => {
      toast.error("Failed to delete schedule.");
      getPostHog()?.capture("mutation_error", {
        action: "schedule_deleted",
        error_message: err instanceof Error ? err.message : "Unknown error",
        schedule_id: id,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
    },
  });
};
