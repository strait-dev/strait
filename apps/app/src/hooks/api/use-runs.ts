import { toast } from "@strait/ui/components/toast";
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
import { DEFAULT_GC_TIME, HIGH_CHURN_STALE_TIME } from "@/hooks/utils";
import { getPostHog } from "@/lib/analytics";
import { apiPath } from "@/lib/api-client.server";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import {
  requireActiveProjectAccess,
  requireActiveProjectAdmin,
} from "@/middlewares/require-access";

const RUN_DETAIL_RETRY_COUNT = 2;
const RUN_DETAIL_RETRY_BASE_MS = 1000;
const RUN_DETAIL_RETRY_MAX_MS = 3000;

const runDetailRetryDelay = (attempt: number) =>
  Math.min(RUN_DETAIL_RETRY_BASE_MS * attempt, RUN_DETAIL_RETRY_MAX_MS);

const fetchRuns = createServerFn({ method: "GET" })
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
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<PaginatedResponse<JobRun>> => {
      await requireActiveProjectAccess(context);
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
    }
  );

const fetchRun = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<JobRun> => {
      await requireActiveProjectAccess(context);
      return await runWithSentryReport(
        apiEffect<JobRun>(apiPath`/v1/runs/${data.id}`)
      );
    }
  );

const fetchRunEvents = createServerFn({ method: "GET" })
  .inputValidator(
    (data: { runId: string; limit?: number; cursor?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<PaginatedResponse<RunEvent>> => {
      await requireActiveProjectAccess(context);
      return await runWithSentryReport(
        apiEffect<PaginatedResponse<RunEvent>>(
          apiPath`/v1/runs/${data.runId}/events`,
          {
            params: { limit: data.limit, cursor: data.cursor },
          }
        )
      );
    }
  );

const replayRunFn = createServerFn({ method: "POST" })
  .inputValidator((data: { runId: string }) => data)
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<JobRun> => {
      await requireActiveProjectAdmin(context);
      return await runWithSentryReport(
        apiEffect<JobRun>(apiPath`/v1/runs/${data.runId}/replay`, {
          method: "POST",
        })
      );
    }
  );

const cancelRunFn = createServerFn({ method: "POST" })
  .inputValidator((data: { runId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }): Promise<void> => {
    await requireActiveProjectAdmin(context);
    return await runWithSentryReport(
      apiEffect<void>(apiPath`/v1/runs/${data.runId}`, { method: "DELETE" })
    );
  });

type RunsSearchParams = ListParams & {
  status?: string;
  job_id?: string;
  search?: string;
};

export const runsQueryOptions = (search?: RunsSearchParams) =>
  queryOptions({
    queryKey: queryKeys.runs.list(search).queryKey,
    queryFn: () => fetchRuns({ data: search ?? {} }),
    staleTime: HIGH_CHURN_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const runQueryOptions = (id: string) =>
  queryOptions({
    queryKey: queryKeys.runs.detail(id).queryKey,
    queryFn: () => fetchRun({ data: { id } }),
    staleTime: HIGH_CHURN_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    retry: RUN_DETAIL_RETRY_COUNT,
    retryDelay: runDetailRetryDelay,
  });

export const runEventsQueryOptions = (runId: string) =>
  queryOptions({
    queryKey: queryKeys.runs.events(runId).queryKey,
    queryFn: () => fetchRunEvents({ data: { runId } }),
    staleTime: HIGH_CHURN_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    retry: RUN_DETAIL_RETRY_COUNT,
    retryDelay: runDetailRetryDelay,
  });

export const useRetryRun = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["runs", "retry"],
    mutationFn: (data: { run_id: string }) =>
      replayRunFn({ data: { runId: data.run_id } }),
    onMutate: async (data) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.runs._def });

      const previousDetail = queryClient.getQueryData<JobRun>(
        queryKeys.runs.detail(data.run_id).queryKey
      );
      const previousLists = queryClient.getQueriesData<
        PaginatedResponse<JobRun>
      >({ queryKey: queryKeys.runs.list._def });

      queryClient.setQueryData<JobRun>(
        queryKeys.runs.detail(data.run_id).queryKey,
        (old) => (old ? { ...old, status: "queued" as const } : old)
      );

      queryClient.setQueriesData<PaginatedResponse<JobRun>>(
        { queryKey: queryKeys.runs.list._def },
        (old) =>
          old
            ? {
                ...old,
                data: old.data.map((run) =>
                  run.id === data.run_id
                    ? { ...run, status: "queued" as const }
                    : run
                ),
              }
            : old
      );

      return { previousDetail, previousLists };
    },
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("run_retried", { run_id: variables.run_id });
    },
    onError: (err, variables, context) => {
      toast.error("Failed to retry run.");
      if (context?.previousDetail) {
        queryClient.setQueryData(
          queryKeys.runs.detail(variables.run_id).queryKey,
          context.previousDetail
        );
      }
      if (context?.previousLists) {
        for (const [key, list] of context.previousLists) {
          queryClient.setQueryData(key, list);
        }
      }
      getPostHog()?.capture("mutation_error", {
        action: "run_retried",
        error_message: err instanceof Error ? err.message : "Unknown error",
        run_id: variables.run_id,
      });
    },
    onSettled: () => {
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
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("run_canceled", { run_id: variables.run_id });
    },
    onMutate: async (data) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.runs._def });

      const previousDetail = queryClient.getQueryData<JobRun>(
        queryKeys.runs.detail(data.run_id).queryKey
      );

      queryClient.setQueryData<JobRun>(
        queryKeys.runs.detail(data.run_id).queryKey,
        (old) => (old ? { ...old, status: "canceled" as const } : old)
      );

      queryClient.setQueriesData<PaginatedResponse<JobRun>>(
        { queryKey: queryKeys.runs.list._def },
        (old) =>
          old
            ? {
                ...old,
                data: old.data.map((run) =>
                  run.id === data.run_id
                    ? { ...run, status: "canceled" as const }
                    : run
                ),
              }
            : old
      );

      return { previousDetail };
    },
    onError: (_err, data, context) => {
      toast.error("Failed to cancel run.");
      if (context?.previousDetail) {
        queryClient.setQueryData(
          queryKeys.runs.detail(data.run_id).queryKey,
          context.previousDetail
        );
      }
      getPostHog()?.capture("mutation_error", {
        action: "run_canceled",
        error_message: _err instanceof Error ? _err.message : "Unknown error",
        run_id: data.run_id,
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
    },
  });
};
