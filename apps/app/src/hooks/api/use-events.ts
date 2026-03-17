import { keepPreviousData, queryOptions } from "@tanstack/react-query";
import type { ListParams } from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { fetchEvent, fetchEvents } from "@/lib/api";

/** List event triggers (durable waits) from /v1/events. */
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

/** Get a single event trigger by key. */
export const eventQueryOptions = (eventKey: string) =>
  queryOptions({
    queryKey: queryKeys.events.detail(eventKey).queryKey,
    queryFn: () => fetchEvent({ data: { eventKey } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });
