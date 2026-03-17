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
  cancelRunFn,
  fetchRun,
  fetchRunEvents,
  fetchRuns,
  replayRunFn,
} from "@/lib/api";

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
