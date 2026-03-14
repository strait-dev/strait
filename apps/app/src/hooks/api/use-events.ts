import { keepPreviousData, queryOptions } from "@tanstack/react-query";
import type {
  ListParams,
  PaginatedResponse,
  RunEvent,
} from "@/hooks/api/types.ts";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils.ts";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const MOCK_EVENTS: RunEvent[] = [
  {
    id: "evt_001",
    run_id: "run_a1",
    type: "log",
    level: "info",
    message: "Job started successfully",
    data: null,
    created_at: "2026-03-14T08:00:00Z",
  },
  {
    id: "evt_002",
    run_id: "run_a1",
    type: "state_change",
    level: "info",
    message: "Status changed from queued to executing",
    data: { from: "queued", to: "executing" },
    created_at: "2026-03-14T08:00:01Z",
  },
  {
    id: "evt_003",
    run_id: "run_a1",
    type: "progress",
    level: "info",
    message: "Processing batch 1 of 5",
    data: { current: 1, total: 5 },
    created_at: "2026-03-14T08:00:10Z",
  },
  {
    id: "evt_004",
    run_id: "run_a2",
    type: "error",
    level: "error",
    message: "Connection refused: endpoint unreachable",
    data: { endpoint: "https://api.example.com/callback" },
    created_at: "2026-03-14T08:01:00Z",
  },
  {
    id: "evt_005",
    run_id: "run_a2",
    type: "log",
    level: "warn",
    message: "Retrying request (attempt 2/3)",
    data: null,
    created_at: "2026-03-14T08:01:05Z",
  },
  {
    id: "evt_006",
    run_id: "run_a3",
    type: "state_change",
    level: "info",
    message: "Status changed from executing to completed",
    data: { from: "executing", to: "completed" },
    created_at: "2026-03-14T08:02:00Z",
  },
  {
    id: "evt_007",
    run_id: "run_a3",
    type: "progress",
    level: "info",
    message: "Processing batch 5 of 5",
    data: { current: 5, total: 5 },
    created_at: "2026-03-14T08:01:55Z",
  },
  {
    id: "evt_008",
    run_id: "run_a4",
    type: "error",
    level: "error",
    message: "Payload validation failed: missing required field 'user_id'",
    data: { field: "user_id" },
    created_at: "2026-03-14T08:03:00Z",
  },
  {
    id: "evt_009",
    run_id: "run_a4",
    type: "log",
    level: "debug",
    message: "Raw payload received",
    data: { size_bytes: 1024 },
    created_at: "2026-03-14T08:02:59Z",
  },
  {
    id: "evt_010",
    run_id: "run_a5",
    type: "state_change",
    level: "info",
    message: "Status changed from queued to canceled",
    data: { from: "queued", to: "canceled", reason: "user_request" },
    created_at: "2026-03-14T08:04:00Z",
  },
];

// ---------------------------------------------------------------------------
// Query helpers
// ---------------------------------------------------------------------------

function filterEvents(
  events: RunEvent[],
  search?: ListParams & { type?: string }
): PaginatedResponse<RunEvent> {
  let filtered = events;

  if (search?.type) {
    filtered = filtered.filter((e) => e.type === search.type);
  }

  if (search?.query) {
    const q = search.query.toLowerCase();
    filtered = filtered.filter((e) => e.message.toLowerCase().includes(q));
  }

  const perPage = search?.per_page ?? 20;
  const page = search?.page ?? 1;
  const start = (page - 1) * perPage;
  const paged = filtered.slice(start, start + perPage);

  return {
    data: paged,
    total_count: filtered.length,
    page_count: Math.ceil(filtered.length / perPage),
  };
}

// ---------------------------------------------------------------------------
// Exports
// ---------------------------------------------------------------------------

/** Query options for listing run events with optional type filter. */
export const eventsQueryOptions = (search?: ListParams & { type?: string }) =>
  queryOptions({
    queryKey: ["events", search ?? {}],
    queryFn: () => Promise.resolve(filterEvents(MOCK_EVENTS, search)),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

/** Query options for a single run event by ID. */
export const eventQueryOptions = (id: string) =>
  queryOptions({
    queryKey: ["events", id],
    queryFn: () => {
      const event = MOCK_EVENTS.find((e) => e.id === id);
      if (!event) {
        throw new Error(`Event not found: ${id}`);
      }
      return Promise.resolve(event);
    },
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });
