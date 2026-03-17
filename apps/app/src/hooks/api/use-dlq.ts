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
  bulkReplayDlqFn,
  cancelRunFn,
  fetchDlqRuns,
  replayDlqRunFn,
} from "@/lib/api";

export const dlqQueryOptions = (search?: ListParams & { search?: string }) =>
  queryOptions({
    queryKey: queryKeys.dlq.list(search).queryKey,
    queryFn: () => fetchDlqRuns({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

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
