import { toast } from "@strait/ui/components/toast";
import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type { JobRun, ListParams, PaginatedResponse } from "@/hooks/api/types";
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

const fetchDlqRuns = createServerFn({ method: "GET" })
  .inputValidator((data: ListParams & { search?: string }) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<PaginatedResponse<JobRun>> => {
      await requireActiveProjectAccess(context);
      return await runWithSentryReport(
        apiEffect<PaginatedResponse<JobRun>>("/v1/runs/dlq", {
          params: {
            limit: data.limit,
            cursor: data.cursor,
            search: data.search,
          },
        })
      );
    }
  );

const replayDlqRunFn = createServerFn({ method: "POST" })
  .inputValidator((data: { runId: string }) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<JobRun> => {
      await requireActiveProjectAdmin(context);
      return await runWithSentryReport(
        apiEffect<JobRun>(apiPath`/v1/runs/${data.runId}/dlq-replay`, {
          method: "POST",
        })
      );
    }
  );

const purgeDlqRunFn = createServerFn({ method: "POST" })
  .inputValidator((data: { runId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<void> => {
    await requireActiveProjectAdmin(context);
    await runWithSentryReport(
      apiEffect(apiPath`/v1/admin/dlq/${data.runId}/purge`, {
        method: "POST",
      })
    );
  });

const bulkReplayDlqFn = createServerFn({ method: "POST" })
  .inputValidator((data: { run_ids: string[] }) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({
      context,
      data,
    }): Promise<{ replayed: JobRun[]; count: number }> => {
      await requireActiveProjectAdmin(context);
      return await runWithSentryReport(
        apiEffect<{ replayed: JobRun[]; count: number }>(
          "/v1/runs/bulk-dlq-replay",
          {
            method: "POST",
            body: { run_ids: data.run_ids },
          }
        )
      );
    }
  );

export const dlqQueryOptions = (search?: ListParams & { search?: string }) =>
  queryOptions({
    queryKey: queryKeys.dlq.list(search).queryKey,
    queryFn: () => fetchDlqRuns({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

const removeIdsFromDlqLists = (
  queryClient: ReturnType<typeof useQueryClient>,
  ids: string[]
) => {
  const idSet = new Set(ids);
  queryClient.setQueriesData<PaginatedResponse<JobRun>>(
    { queryKey: queryKeys.dlq.list._def },
    (old) =>
      old ? { ...old, data: old.data.filter((r) => !idSet.has(r.id)) } : old
  );
};

const snapshotDlqLists = (queryClient: ReturnType<typeof useQueryClient>) =>
  queryClient.getQueriesData<PaginatedResponse<JobRun>>({
    queryKey: queryKeys.dlq.list._def,
  });

const restoreDlqLists = (
  queryClient: ReturnType<typeof useQueryClient>,
  snapshot: ReturnType<typeof snapshotDlqLists>
) => {
  for (const [key, data] of snapshot) {
    queryClient.setQueryData(key, data);
  }
};

export const useRetryDlqItem = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["dlq", "retry"],
    mutationFn: (data: { id: string }) =>
      replayDlqRunFn({ data: { runId: data.id } }),
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.dlq.list._def });
      const previousLists = snapshotDlqLists(queryClient);
      removeIdsFromDlqLists(queryClient, [variables.id]);
      return { previousLists };
    },
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("dlq_item_retried", { run_id: variables.id });
    },
    onError: (err, variables, context) => {
      toast.error("Failed to retry dead-letter run.");
      if (context?.previousLists) {
        restoreDlqLists(queryClient, context.previousLists);
      }
      getPostHog()?.capture("mutation_error", {
        action: "dlq_item_retried",
        error_message: err instanceof Error ? err.message : "Unknown error",
        run_id: variables.id,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.dlq._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
    },
  });
};

export const useDiscardDlqItem = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["dlq", "discard"],
    mutationFn: (data: { id: string }) =>
      purgeDlqRunFn({ data: { runId: data.id } }),
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.dlq.list._def });
      const previousLists = snapshotDlqLists(queryClient);
      removeIdsFromDlqLists(queryClient, [variables.id]);
      return { previousLists };
    },
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("dlq_item_discarded", { run_id: variables.id });
    },
    onError: (err, variables, context) => {
      toast.error("Failed to discard dead-letter run.");
      if (context?.previousLists) {
        restoreDlqLists(queryClient, context.previousLists);
      }
      getPostHog()?.capture("mutation_error", {
        action: "dlq_item_discarded",
        error_message: err instanceof Error ? err.message : "Unknown error",
        run_id: variables.id,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.dlq._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
    },
  });
};

export const useBulkRetryDlq = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["dlq", "bulkRetry"],
    mutationFn: (data: { ids: string[] }) =>
      bulkReplayDlqFn({ data: { run_ids: data.ids } }),
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.dlq.list._def });
      const previousLists = snapshotDlqLists(queryClient);
      removeIdsFromDlqLists(queryClient, variables.ids);
      return { previousLists };
    },
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("dlq_bulk_retried", {
        count: variables.ids.length,
      });
    },
    onError: (err, variables, context) => {
      toast.error("Failed to retry dead-letter runs.");
      if (context?.previousLists) {
        restoreDlqLists(queryClient, context.previousLists);
      }
      getPostHog()?.capture("mutation_error", {
        action: "dlq_bulk_retried",
        error_message: err instanceof Error ? err.message : "Unknown error",
        count: variables.ids.length,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.dlq._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
    },
  });
};

export const useBulkDiscardDlq = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["dlq", "bulkDiscard"],
    mutationFn: async (data: { ids: string[] }) => {
      await Promise.all(
        data.ids.map((id) => purgeDlqRunFn({ data: { runId: id } }))
      );
      return { canceled: data.ids.length };
    },
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.dlq.list._def });
      const previousLists = snapshotDlqLists(queryClient);
      removeIdsFromDlqLists(queryClient, variables.ids);
      return { previousLists };
    },
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("dlq_bulk_discarded", {
        count: variables.ids.length,
      });
    },
    onError: (err, variables, context) => {
      toast.error("Failed to discard dead-letter runs.");
      if (context?.previousLists) {
        restoreDlqLists(queryClient, context.previousLists);
      }
      getPostHog()?.capture("mutation_error", {
        action: "dlq_bulk_discarded",
        error_message: err instanceof Error ? err.message : "Unknown error",
        count: variables.ids.length,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.dlq._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
    },
  });
};
