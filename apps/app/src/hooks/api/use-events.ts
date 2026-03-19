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

// ---------------------------------------------------------------------------
// Server functions
// ---------------------------------------------------------------------------

export const fetchEvents = createServerFn({ method: "GET" })
  .inputValidator(
    (data: ListParams & { type?: string; search?: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<PaginatedResponse<EventTrigger>>("/v1/events", {
        params: {
          limit: data.limit,
          cursor: data.cursor,
          type: data.type,
          search: data.search,
        },
      })
    );
  });

export const fetchEvent = createServerFn({ method: "GET" })
  .inputValidator((data: { eventKey: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    return await runWithSentryReport(
      apiEffect<EventTrigger>(`/v1/events/${data.eventKey}`)
    );
  });

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

export const eventsQueryOptions = (
  search?: ListParams & { type?: string; search?: string }
) =>
  queryOptions({
    queryKey: queryKeys.events.list(search).queryKey,
    queryFn: () => fetchEvents({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const eventQueryOptions = (eventKey: string) =>
  queryOptions({
    queryKey: queryKeys.events.detail(eventKey).queryKey,
    queryFn: () => fetchEvent({ data: { eventKey } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });
