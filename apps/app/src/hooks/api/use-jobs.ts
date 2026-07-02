import { toast } from "@strait/ui/components/toast";
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
import { apiPath } from "@/lib/api-client.server";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import {
  requireActiveProjectAccess,
  requireActiveProjectAdmin,
} from "@/middlewares/require-access";

export type JobMutationInput = {
  name?: string;
  slug?: string;
  description?: string;
  endpoint_url?: string;
  cron?: string;
  max_attempts?: number;
  timeout_secs?: number;
  retry_strategy?: "exponential" | "linear" | "fixed" | "custom";
  execution_mode?: "http" | "worker";
  queue_name?: string;
  enabled?: boolean;
};

export type CreateJobInput = JobMutationInput & {
  name: string;
};

export type UpdateJobInput = JobMutationInput & {
  id: string;
};

function slugFromName(name: string) {
  const base = name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 48);
  const suffix = Math.random().toString(36).slice(2, 8);
  return `${base || "job"}-${suffix}`;
}

export const fetchJobs = createServerFn({ method: "GET" })
  .inputValidator(
    (data: ListParams & { status?: string; search?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<PaginatedResponse<Job>> => {
      await requireActiveProjectAccess(context);
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
    }
  );

export const fetchJob = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<Job> => {
      await requireActiveProjectAccess(context);
      return await runWithSentryReport(
        apiEffect<Job>(apiPath`/v1/jobs/${data.id}`)
      );
    }
  );

export const triggerJobFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { id: string; payload?: unknown; priority?: number }) => data
  )
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<JobRun> => {
      await requireActiveProjectAdmin(context);
      return await runWithSentryReport(
        apiEffect<JobRun>(apiPath`/v1/jobs/${data.id}/trigger`, {
          method: "POST",
          body: { payload: data.payload, priority: data.priority },
        })
      );
    }
  );

export const createJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: CreateJobInput) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<Job> => {
      const projectId = await requireActiveProjectAdmin(context);
      const executionMode = data.execution_mode ?? "http";
      return await runWithSentryReport(
        apiEffect<Job>("/v1/jobs", {
          method: "POST",
          body: {
            project_id: projectId,
            name: data.name,
            slug: data.slug || slugFromName(data.name),
            description: data.description,
            endpoint_url:
              executionMode === "worker" ? undefined : data.endpoint_url,
            cron: data.cron,
            max_attempts: data.max_attempts,
            timeout_secs: data.timeout_secs,
            retry_strategy: data.retry_strategy,
            execution_mode: executionMode,
            queue_name: data.queue_name,
            enabled: data.enabled,
          },
        })
      );
    }
  );

export const updateJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: UpdateJobInput) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<Job> => {
      await requireActiveProjectAdmin(context);
      const { id, ...body } = data;
      return await runWithSentryReport(
        apiEffect<Job>(apiPath`/v1/jobs/${id}`, { method: "PATCH", body })
      );
    }
  );

export const deleteJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<void> => {
    await requireActiveProjectAdmin(context);
    return await runWithSentryReport(
      apiEffect<void>(apiPath`/v1/jobs/${data.id}`, { method: "DELETE" })
    );
  });

export const fetchJobHealth = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string; window?: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<JobHealthResponse> => {
    await requireActiveProjectAccess(context);
    return await runWithSentryReport(
      apiEffect<JobHealthResponse>(apiPath`/v1/jobs/${data.id}/health`, {
        params: { window: data.window },
      })
    );
  });

export const pauseJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string; reason?: string }) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<Job> => {
      await requireActiveProjectAdmin(context);
      return await runWithSentryReport(
        apiEffect<Job>(apiPath`/v1/jobs/${data.id}/pause`, {
          method: "POST",
          body: { reason: data.reason },
        })
      );
    }
  );

export const resumeJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<Job> => {
      await requireActiveProjectAdmin(context);
      return await runWithSentryReport(
        apiEffect<Job>(apiPath`/v1/jobs/${data.id}/resume`, { method: "POST" })
      );
    }
  );

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
      toast.error("Failed to trigger job.");
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

export const useCreateJob = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["jobs", "create"],
    mutationFn: async (data: CreateJobInput) =>
      (await createJobFn({ data })) as Job,
    onSuccess: (job) => {
      getPostHog()?.capture("job_created", { job_id: job.id });
    },
    onError: (err) => {
      getPostHog()?.capture("mutation_error", {
        action: "job_created",
        error_message: err instanceof Error ? err.message : "Unknown error",
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
    },
  });
};

export const useUpdateJob = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["jobs", "update"],
    mutationFn: async (data: UpdateJobInput) =>
      (await updateJobFn({ data })) as Job,
    onSuccess: (job) => {
      getPostHog()?.capture("job_updated", { job_id: job.id });
    },
    onError: (err, variables) => {
      getPostHog()?.capture("mutation_error", {
        action: "job_updated",
        error_message: err instanceof Error ? err.message : "Unknown error",
        job_id: variables.id,
      });
    },
    onSettled: (_data, _err, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
      if (variables?.id) {
        queryClient.invalidateQueries({
          queryKey: queryKeys.jobs.detail(variables.id).queryKey,
        });
      }
    },
  });
};

export const useDeleteJob = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["jobs", "delete"],
    mutationFn: (id: string) => deleteJobFn({ data: { id } }),
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.jobs._def });
      const previousLists = queryClient.getQueriesData<PaginatedResponse<Job>>({
        queryKey: queryKeys.jobs.list._def,
      });

      queryClient.setQueriesData<PaginatedResponse<Job>>(
        { queryKey: queryKeys.jobs.list._def },
        (old) =>
          old ? { ...old, data: old.data.filter((job) => job.id !== id) } : old
      );

      return { previousLists };
    },
    onSuccess: (_data, id) => {
      getPostHog()?.capture("job_deleted", { job_id: id });
    },
    onError: (err, variables, context) => {
      toast.error("Failed to delete job.");
      if (context?.previousLists) {
        for (const [key, data] of context.previousLists) {
          queryClient.setQueryData(key, data);
        }
      }
      getPostHog()?.capture("mutation_error", {
        action: "job_deleted",
        error_message: err instanceof Error ? err.message : "Unknown error",
        job_id: variables,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
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
        (old) => (old ? { ...old, paused: true } : old)
      );

      queryClient.setQueriesData<PaginatedResponse<Job>>(
        { queryKey: queryKeys.jobs.list._def },
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

      return { previousDetail };
    },
    onError: (_err, data, context) => {
      toast.error("Failed to pause job.");
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
        (old) => (old ? { ...old, paused: false } : old)
      );

      queryClient.setQueriesData<PaginatedResponse<Job>>(
        { queryKey: queryKeys.jobs.list._def },
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

      return { previousDetail };
    },
    onError: (_err, data, context) => {
      toast.error("Failed to resume job.");
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
