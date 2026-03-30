import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type {
  Job,
  JobHealthResponse,
  JobRun,
  ListParams,
  PaginatedResponse,
} from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getPostHog } from "@/lib/analytics";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

export const fetchJobs = createServerFn({ method: "GET" })
  .inputValidator(
    (data: ListParams & { status?: string; search?: string }) => data
  )
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<PaginatedResponse<Job>> => {
    return await runWithSentryReport(
      apiEffect<PaginatedResponse<Job>>("/v1/jobs", {
        params: {
          limit: data.limit,
          cursor: data.cursor,
          status: data.status,
          search: data.search,
        },
      })
    );
  });

export const fetchJob = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<Job> => {
    return await runWithSentryReport(apiEffect<Job>(`/v1/jobs/${data.id}`));
  });

export const triggerJobFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { id: string; payload?: unknown; priority?: number }) => data
  )
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<JobRun> => {
    return await runWithSentryReport(
      apiEffect<JobRun>(`/v1/jobs/${data.id}/trigger`, {
        method: "POST",
        body: { payload: data.payload, priority: data.priority },
      })
    );
  });

export const updateJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string; enabled?: boolean }) => data)
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<Job> => {
    const { id, ...body } = data;
    return await runWithSentryReport(
      apiEffect<Job>(`/v1/jobs/${id}`, { method: "PATCH", body })
    );
  });

export const fetchJobHealth = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string; window?: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<JobHealthResponse> => {
    return await runWithSentryReport(
      apiEffect<JobHealthResponse>(`/v1/jobs/${data.id}/health`, {
        params: { window: data.window },
      })
    );
  });

export const deleteJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<void> => {
    return await runWithSentryReport(
      apiEffect<void>(`/v1/jobs/${data.id}`, { method: "DELETE" })
    );
  });

export const pauseJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string; reason?: string }) => data)
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<Job> => {
    return await runWithSentryReport(
      apiEffect<Job>(`/v1/jobs/${data.id}/pause`, {
        method: "POST",
        body: { reason: data.reason },
      })
    );
  });

export const resumeJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  // @ts-expect-error tsgo cannot resolve createServerFn handler generics
  .handler(async ({ data }): Promise<Job> => {
    return await runWithSentryReport(
      apiEffect<Job>(`/v1/jobs/${data.id}/resume`, { method: "POST" })
    );
  });

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

export const jobHealthQueryOptions = (id: string, window = "7d") =>
  queryOptions({
    queryKey: [...queryKeys.jobs.detail(id).queryKey, "health", window],
    queryFn: () => fetchJobHealth({ data: { id, window } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const useTriggerJob = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["jobs", "trigger"],
    mutationFn: (data: { id: string; payload?: unknown }) =>
      triggerJobFn({ data }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("job_triggered", { job_id: variables.id });
    },
    onError: (err, variables) => {
      getPostHog()?.capture("mutation_error", {
        action: "job_triggered",
        error_message: err instanceof Error ? err.message : "Unknown error",
        job_id: variables.id,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
    },
  });
};

export const usePauseJob = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["jobs", "pause"],
    mutationFn: (data: { id: string; reason?: string }) => pauseJobFn({ data }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("job_paused", { job_id: variables.id });
    },
    onMutate: async (data) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.jobs._def });

      const previousDetail = queryClient.getQueryData<Job>(
        queryKeys.jobs.detail(data.id).queryKey
      );

      queryClient.setQueryData<Job>(
        queryKeys.jobs.detail(data.id).queryKey,
        (old) => (old ? { ...old, enabled: false } : old)
      );

      queryClient.setQueriesData<PaginatedResponse<Job>>(
        { queryKey: queryKeys.jobs.list._def },
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

      return { previousDetail };
    },
    onError: (_err, data, context) => {
      if (context?.previousDetail) {
        queryClient.setQueryData(
          queryKeys.jobs.detail(data.id).queryKey,
          context.previousDetail
        );
      }
      getPostHog()?.capture("mutation_error", {
        action: "job_paused",
        error_message: _err instanceof Error ? _err.message : "Unknown error",
        job_id: data.id,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
    },
  });
};

export const useResumeJob = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["jobs", "resume"],
    mutationFn: (data: { id: string }) => resumeJobFn({ data }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("job_resumed", { job_id: variables.id });
    },
    onMutate: async (data) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.jobs._def });

      const previousDetail = queryClient.getQueryData<Job>(
        queryKeys.jobs.detail(data.id).queryKey
      );

      queryClient.setQueryData<Job>(
        queryKeys.jobs.detail(data.id).queryKey,
        (old) => (old ? { ...old, enabled: true } : old)
      );

      queryClient.setQueriesData<PaginatedResponse<Job>>(
        { queryKey: queryKeys.jobs.list._def },
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

      return { previousDetail };
    },
    onError: (_err, data, context) => {
      if (context?.previousDetail) {
        queryClient.setQueryData(
          queryKeys.jobs.detail(data.id).queryKey,
          context.previousDetail
        );
      }
      getPostHog()?.capture("mutation_error", {
        action: "job_resumed",
        error_message: _err instanceof Error ? _err.message : "Unknown error",
        job_id: data.id,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
    },
  });
};

export const useDeleteJob = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["jobs", "delete"],
    mutationFn: (data: { id: string }) => deleteJobFn({ data }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("job_deleted", { job_id: variables.id });
    },
    onError: (err, variables) => {
      getPostHog()?.capture("mutation_error", {
        action: "job_deleted",
        error_message: err instanceof Error ? err.message : "Unknown error",
        job_id: variables.id,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
    },
  });
};
