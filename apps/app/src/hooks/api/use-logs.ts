import { keepPreviousData, queryOptions } from "@tanstack/react-query";
import type {
  ListParams,
  PaginatedResponse,
  RunEvent,
} from "@/hooks/api/types.ts";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils.ts";

// ---------------------------------------------------------------------------
// Mock data — logs are RunEvents with type "log"
// ---------------------------------------------------------------------------

const MOCK_LOGS: RunEvent[] = [
  {
    id: "log_001",
    run_id: "run_a1",
    type: "log",
    level: "info",
    message: "Job started successfully",
    data: null,
    created_at: "2026-03-14T08:00:00Z",
  },
  {
    id: "log_002",
    run_id: "run_a1",
    type: "log",
    level: "debug",
    message: "Resolved endpoint: https://api.example.com/webhook",
    data: { endpoint: "https://api.example.com/webhook" },
    created_at: "2026-03-14T08:00:01Z",
  },
  {
    id: "log_003",
    run_id: "run_a1",
    type: "log",
    level: "info",
    message: "Payload validated against schema",
    data: null,
    created_at: "2026-03-14T08:00:02Z",
  },
  {
    id: "log_004",
    run_id: "run_a2",
    type: "log",
    level: "warn",
    message: "Retrying request (attempt 2/3)",
    data: { attempt: 2, max_attempts: 3 },
    created_at: "2026-03-14T08:01:05Z",
  },
  {
    id: "log_005",
    run_id: "run_a2",
    type: "log",
    level: "error",
    message: "All retry attempts exhausted",
    data: { attempts: 3 },
    created_at: "2026-03-14T08:01:20Z",
  },
  {
    id: "log_006",
    run_id: "run_a3",
    type: "log",
    level: "info",
    message: "Dispatched to worker pool",
    data: { pool: "default", worker_id: "w-07" },
    created_at: "2026-03-14T08:02:00Z",
  },
  {
    id: "log_007",
    run_id: "run_a3",
    type: "log",
    level: "debug",
    message: "Execution trace recorded",
    data: { total_ms: 342 },
    created_at: "2026-03-14T08:02:01Z",
  },
  {
    id: "log_008",
    run_id: "run_a3",
    type: "log",
    level: "info",
    message: "Job completed with result",
    data: { result_size_bytes: 512 },
    created_at: "2026-03-14T08:02:02Z",
  },
  {
    id: "log_009",
    run_id: "run_a4",
    type: "log",
    level: "error",
    message: "Payload validation failed: missing required field 'user_id'",
    data: { field: "user_id" },
    created_at: "2026-03-14T08:03:00Z",
  },
  {
    id: "log_010",
    run_id: "run_a4",
    type: "log",
    level: "warn",
    message: "Fallback endpoint not configured",
    data: null,
    created_at: "2026-03-14T08:03:01Z",
  },
  {
    id: "log_011",
    run_id: "run_a5",
    type: "log",
    level: "info",
    message: "Run canceled by user",
    data: { canceled_by: "user_001" },
    created_at: "2026-03-14T08:04:00Z",
  },
  {
    id: "log_012",
    run_id: "run_a5",
    type: "log",
    level: "debug",
    message: "Cleanup completed for canceled run",
    data: null,
    created_at: "2026-03-14T08:04:01Z",
  },
];

// ---------------------------------------------------------------------------
// Query helpers
// ---------------------------------------------------------------------------

function filterLogs(
  logs: RunEvent[],
  search?: ListParams & { level?: string; run_id?: string }
): PaginatedResponse<RunEvent> {
  let filtered = logs;

  if (search?.level) {
    filtered = filtered.filter((l) => l.level === search.level);
  }

  if (search?.run_id) {
    filtered = filtered.filter((l) => l.run_id === search.run_id);
  }

  if (search?.query) {
    const q = search.query.toLowerCase();
    filtered = filtered.filter((l) => l.message.toLowerCase().includes(q));
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

/** Query options for listing log entries with optional level and run_id filters. */
export const logsQueryOptions = (
  search?: ListParams & { level?: string; run_id?: string }
) =>
  queryOptions({
    queryKey: ["logs", search ?? {}],
    queryFn: () => Promise.resolve(filterLogs(MOCK_LOGS, search)),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });
