import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type { JobRun, ListParams, PaginatedResponse } from "@/hooks/api/types";
import { cancelRunFn } from "@/hooks/api/use-runs";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiRequest } from "@/lib/api-client.server";
import { authMiddleware } from "@/middlewares/auth";

// ---------------------------------------------------------------------------
// Server functions
// ---------------------------------------------------------------------------

export const fetchDlqRuns = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams & { search?: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await apiRequest<PaginatedResponse<JobRun>>("/v1/runs/dlq", {
      params: { limit: data.limit, cursor: data.cursor, search: data.search },
    });
  });

export const replayDlqRunFn = createServerFn({ method: "POST" })
  .inputValidator((data: { runId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await apiRequest<{ id: string }>(`/v1/runs/${data.runId}/dlq-replay`, {
      method: "POST",
    });
  });

export const bulkReplayDlqFn = createServerFn({ method: "POST" })
  .inputValidator((data: { run_ids: string[] }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await apiRequest<{ replayed: number }>("/v1/runs/bulk-dlq-replay", {
      method: "POST",
      body: { run_ids: data.run_ids },
    });
  });

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

export const dlqQueryOptions = (search?: ListParams & { search?: string }) =>
  queryOptions({
    queryKey: queryKeys.dlq.list(search).queryKey,
    queryFn: () => fetchDlqRuns({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export const useRetryDlqItem = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["dlq", "retry"],
    mutationFn: (data: { id: string }) =>
      replayDlqRunFn({ data: { runId: data.id } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.dlq._def });
    },
  });
};

export const useDiscardDlqItem = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["dlq", "discard"],
    mutationFn: (data: { id: string }) =>
      cancelRunFn({ data: { runId: data.id } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.dlq._def });
    },
  });
};

export const useBulkRetryDlq = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["dlq", "bulkRetry"],
    mutationFn: (data: { ids: string[] }) =>
      bulkReplayDlqFn({ data: { run_ids: data.ids } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.dlq._def });
    },
  });
};

export const useBulkDiscardDlq = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["dlq", "bulkDiscard"],
    mutationFn: async (data: { ids: string[] }) => {
      await Promise.all(
        data.ids.map((id) => cancelRunFn({ data: { runId: id } }))
      );
      return { discarded: data.ids.length };
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.dlq._def });
    },
  });
};
