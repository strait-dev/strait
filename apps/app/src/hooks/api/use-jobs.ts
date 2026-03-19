import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type { Job, ListParams, PaginatedResponse } from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

// ---------------------------------------------------------------------------
// Server functions
// ---------------------------------------------------------------------------

export const fetchJobs = createServerFn({ method: "GET" })
  .inputValidator(
    (data: ListParams & { status?: string; search?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
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
  .handler(async ({ data }) => {
    return await runWithSentryReport(apiEffect<Job>(`/v1/jobs/${data.id}`));
  });

export const triggerJobFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { id: string; payload?: unknown; priority?: number }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<{ id: string }>(`/v1/jobs/${data.id}/trigger`, {
        method: "POST",
        body: { payload: data.payload, priority: data.priority },
      })
    );
  });

export const updateJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string; enabled?: boolean }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const { id, ...body } = data;
    return await runWithSentryReport(
      apiEffect<Job>(`/v1/jobs/${id}`, { method: "PATCH", body })
    );
  });

export const deleteJobFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<void>(`/v1/jobs/${data.id}`, { method: "DELETE" })
    );
  });

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

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
