import { keepPreviousData, queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type {
  EventTrigger,
  ListParams,
  PaginatedResponse,
} from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveProjectAccess } from "@/middlewares/require-access";

const fetchEvents = createServerFn({ method: "GET" })
  .inputValidator(
    (
      data: ListParams & {
        status?: string;
        workflow_run_id?: string;
        source_type?: string;
      }
    ) => data
  )
  .middleware([authMiddleware])
  .handler(
    // @ts-expect-error tsgo cannot resolve createServerFn handler generics
    async ({ context, data }): Promise<PaginatedResponse<EventTrigger>> => {
      await requireActiveProjectAccess(context);
      return await runWithSentryReport(
        apiEffect<PaginatedResponse<EventTrigger>>("/v1/events", {
          params: {
            limit: data.limit,
            cursor: data.cursor,
            status: data.status,
            workflow_run_id: data.workflow_run_id,
            source_type: data.source_type,
          },
        })
      );
    }
  );

export const eventsQueryOptions = (
  search?: ListParams & {
    status?: string;
    workflow_run_id?: string;
    source_type?: string;
  }
) =>
  queryOptions({
    queryKey: queryKeys.events.list(search).queryKey,
    queryFn: () => fetchEvents({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });
