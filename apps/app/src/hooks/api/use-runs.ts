import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type {
  JobRun,
  ListParams,
  PaginatedResponse,
  RunEvent,
} from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

// ---------------------------------------------------------------------------
// Server functions
// ---------------------------------------------------------------------------

export const fetchRuns = createServerFn({ method: "GET" })
  .inputValidator(
    (
      data: ListParams & {
        status?: string;
        job_id?: string;
        search?: string;
      }
    ) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<PaginatedResponse<JobRun>>("/v1/runs", {
        params: {
          limit: data.limit,
          cursor: data.cursor,
          status: data.status,
          job_id: data.job_id,
          search: data.search,
        },
      })
    );
  });

export const fetchRun = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(apiEffect<JobRun>(`/v1/runs/${data.id}`));
  });

export const fetchRunEvents = createServerFn({ method: "GET" })
  .inputValidator(
    (data: { runId: string; limit?: number; cursor?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<PaginatedResponse<RunEvent>>(`/v1/runs/${data.runId}/events`, {
        params: { limit: data.limit, cursor: data.cursor },
      })
    );
  });

export const replayRunFn = createServerFn({ method: "POST" })
  .inputValidator((data: { runId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<{ id: string }>(`/v1/runs/${data.runId}/replay`, {
        method: "POST",
      })
    );
  });

export const cancelRunFn = createServerFn({ method: "POST" })
  .inputValidator((data: { runId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<void>(`/v1/runs/${data.runId}`, { method: "DELETE" })
    );
  });

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

type RunsSearchParams = ListParams & {
  status?: string;
  job_id?: string;
  search?: string;
};

export const runsQueryOptions = (search?: RunsSearchParams) =>
  queryOptions({
    queryKey: queryKeys.runs.list(search).queryKey,
    queryFn: () => fetchRuns({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const runQueryOptions = (id: string) =>
  queryOptions({
    queryKey: queryKeys.runs.detail(id).queryKey,
    queryFn: () => fetchRun({ data: { id } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const runEventsQueryOptions = (runId: string) =>
  queryOptions({
    queryKey: queryKeys.runs.events(runId).queryKey,
    queryFn: () => fetchRunEvents({ data: { runId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export const useRetryRun = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["runs", "retry"],
    mutationFn: (data: { run_id: string }) =>
      replayRunFn({ data: { runId: data.run_id } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
    },
  });
};

export const useCancelRun = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["runs", "cancel"],
    mutationFn: (data: { run_id: string }) =>
      cancelRunFn({ data: { runId: data.run_id } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
    },
  });
};
