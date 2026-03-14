import { queryOptions, useMutation } from "@tanstack/react-query";
import type {
  JobRun,
  ListParams,
  PaginatedResponse,
  RunEvent,
  RunStatus,
} from "@/hooks/api/types.ts";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils.ts";

type RunsSearchParams = ListParams & { status?: RunStatus[] };

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const BASE_DATE = "2026-03-14T08:00:00Z";

/** Offset a base ISO date by `minutes`. */
const offset = (minutes: number): string =>
  new Date(new Date(BASE_DATE).getTime() + minutes * 60_000).toISOString();

const MOCK_RUNS: JobRun[] = [
  {
    id: "run_01",
    job_id: "job_process_payments",
    project_id: "proj_1",
    tags: { env: "production" },
    status: "completed",
    attempt: 1,
    payload: { batch_size: 500 },
    result: { processed: 500, failed: 0 },
    metadata: {},
    error: "",
    triggered_by: "cron",
    scheduled_at: offset(0),
    started_at: offset(1),
    finished_at: offset(5),
    heartbeat_at: offset(4),
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "",
    priority: 10,
    idempotency_key: "pay-20260314-001",
    job_version: 3,
    job_version_id: "ver_pay_3",
    workflow_step_run_id: "",
    max_attempts_override: 0,
    timeout_secs_override: 0,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 1,
    retry_max_delay_secs: 60,
    execution_trace: {
      queue_wait_ms: 120,
      dequeue_ms: 5,
      connect_ms: 30,
      ttfb_ms: 80,
      transfer_ms: 200,
      total_ms: 4200,
      dispatch_ms: 10,
    },
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 0,
    created_by: "system",
    batch_id: "",
    concurrency_key: "",
    created_at: offset(0),
  },
  {
    id: "run_02",
    job_id: "job_send_email_batch",
    project_id: "proj_1",
    tags: { env: "production", campaign: "march" },
    status: "executing",
    attempt: 1,
    payload: { template: "welcome", count: 1200 },
    result: null,
    metadata: { progress: "45%" },
    error: "",
    triggered_by: "manual",
    scheduled_at: offset(10),
    started_at: offset(11),
    finished_at: null,
    heartbeat_at: offset(14),
    next_retry_at: null,
    expires_at: offset(71),
    parent_run_id: "",
    priority: 5,
    idempotency_key: "email-batch-march-001",
    job_version: 1,
    job_version_id: "ver_email_1",
    workflow_step_run_id: "",
    max_attempts_override: 0,
    timeout_secs_override: 0,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 5,
    retry_max_delay_secs: 5,
    execution_trace: null,
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 0,
    created_by: "user_42",
    batch_id: "batch_email_01",
    concurrency_key: "email-send",
    created_at: offset(10),
  },
  {
    id: "run_03",
    job_id: "job_sync_inventory",
    project_id: "proj_1",
    tags: { env: "production" },
    status: "failed",
    attempt: 3,
    payload: { warehouse: "us-east-1" },
    result: null,
    metadata: {},
    error: "connection refused: inventory-service:8443",
    triggered_by: "cron",
    scheduled_at: offset(-60),
    started_at: offset(-59),
    finished_at: offset(-55),
    heartbeat_at: offset(-56),
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "",
    priority: 8,
    idempotency_key: "sync-inv-20260314",
    job_version: 2,
    job_version_id: "ver_inv_2",
    workflow_step_run_id: "",
    max_attempts_override: 3,
    timeout_secs_override: 120,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 2,
    retry_max_delay_secs: 30,
    execution_trace: {
      queue_wait_ms: 50,
      dequeue_ms: 3,
      connect_ms: 5000,
      ttfb_ms: 0,
      transfer_ms: 0,
      total_ms: 5100,
      dispatch_ms: 8,
    },
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 0,
    created_by: "system",
    batch_id: "",
    concurrency_key: "",
    created_at: offset(-60),
  },
  {
    id: "run_04",
    job_id: "job_generate_report",
    project_id: "proj_1",
    tags: { env: "staging", report: "monthly" },
    status: "queued",
    attempt: 1,
    payload: { month: "2026-02", format: "pdf" },
    result: null,
    metadata: {},
    error: "",
    triggered_by: "manual",
    scheduled_at: offset(20),
    started_at: null,
    finished_at: null,
    heartbeat_at: null,
    next_retry_at: null,
    expires_at: offset(80),
    parent_run_id: "",
    priority: 3,
    idempotency_key: "report-2026-02",
    job_version: 1,
    job_version_id: "ver_report_1",
    workflow_step_run_id: "",
    max_attempts_override: 0,
    timeout_secs_override: 0,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 10,
    retry_max_delay_secs: 10,
    execution_trace: null,
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 0,
    created_by: "user_42",
    batch_id: "",
    concurrency_key: "",
    created_at: offset(19),
  },
  {
    id: "run_05",
    job_id: "job_backup_database",
    project_id: "proj_1",
    tags: { env: "production", db: "primary" },
    status: "completed",
    attempt: 1,
    payload: { target: "s3://backups/daily" },
    result: { size_mb: 2048, duration_secs: 340 },
    metadata: {},
    error: "",
    triggered_by: "cron",
    scheduled_at: offset(-120),
    started_at: offset(-119),
    finished_at: offset(-113),
    heartbeat_at: offset(-114),
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "",
    priority: 10,
    idempotency_key: "backup-20260314",
    job_version: 5,
    job_version_id: "ver_backup_5",
    workflow_step_run_id: "",
    max_attempts_override: 0,
    timeout_secs_override: 600,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 5,
    retry_max_delay_secs: 120,
    execution_trace: {
      queue_wait_ms: 80,
      dequeue_ms: 4,
      connect_ms: 15,
      ttfb_ms: 200,
      transfer_ms: 339_000,
      total_ms: 340_000,
      dispatch_ms: 6,
    },
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 0,
    created_by: "system",
    batch_id: "",
    concurrency_key: "db-backup",
    created_at: offset(-120),
  },
  {
    id: "run_06",
    job_id: "job_cleanup_temp_files",
    project_id: "proj_1",
    tags: { env: "production" },
    status: "completed",
    attempt: 1,
    payload: { older_than_hours: 24 },
    result: { deleted: 137 },
    metadata: {},
    error: "",
    triggered_by: "cron",
    scheduled_at: offset(-180),
    started_at: offset(-179),
    finished_at: offset(-178),
    heartbeat_at: offset(-178),
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "",
    priority: 1,
    idempotency_key: "cleanup-20260314",
    job_version: 1,
    job_version_id: "ver_cleanup_1",
    workflow_step_run_id: "",
    max_attempts_override: 0,
    timeout_secs_override: 0,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 5,
    retry_max_delay_secs: 5,
    execution_trace: {
      queue_wait_ms: 30,
      dequeue_ms: 2,
      connect_ms: 10,
      ttfb_ms: 50,
      transfer_ms: 800,
      total_ms: 1200,
      dispatch_ms: 3,
    },
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 0,
    created_by: "system",
    batch_id: "",
    concurrency_key: "",
    created_at: offset(-180),
  },
  {
    id: "run_07",
    job_id: "job_process_payments",
    project_id: "proj_1",
    tags: { env: "production" },
    status: "timed_out",
    attempt: 2,
    payload: { batch_size: 1000 },
    result: null,
    metadata: {},
    error: "execution exceeded 300s timeout",
    triggered_by: "cron",
    scheduled_at: offset(-240),
    started_at: offset(-239),
    finished_at: offset(-234),
    heartbeat_at: offset(-237),
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "",
    priority: 10,
    idempotency_key: "pay-20260313-002",
    job_version: 3,
    job_version_id: "ver_pay_3",
    workflow_step_run_id: "",
    max_attempts_override: 0,
    timeout_secs_override: 300,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 1,
    retry_max_delay_secs: 60,
    execution_trace: {
      queue_wait_ms: 200,
      dequeue_ms: 6,
      connect_ms: 25,
      ttfb_ms: 100,
      transfer_ms: 0,
      total_ms: 300_000,
      dispatch_ms: 12,
    },
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 0,
    created_by: "system",
    batch_id: "",
    concurrency_key: "",
    created_at: offset(-240),
  },
  {
    id: "run_08",
    job_id: "job_send_email_batch",
    project_id: "proj_1",
    tags: { env: "production", campaign: "february" },
    status: "dead_letter",
    attempt: 5,
    payload: { template: "promo", count: 50 },
    result: null,
    metadata: {},
    error: "max attempts exhausted; last error: SMTP 550 mailbox full",
    triggered_by: "retry",
    scheduled_at: offset(-300),
    started_at: offset(-299),
    finished_at: offset(-298),
    heartbeat_at: offset(-298),
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "run_prev_email",
    priority: 5,
    idempotency_key: "email-promo-feb-005",
    job_version: 1,
    job_version_id: "ver_email_1",
    workflow_step_run_id: "",
    max_attempts_override: 5,
    timeout_secs_override: 0,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 10,
    retry_max_delay_secs: 300,
    execution_trace: {
      queue_wait_ms: 60,
      dequeue_ms: 3,
      connect_ms: 20,
      ttfb_ms: 500,
      transfer_ms: 100,
      total_ms: 1500,
      dispatch_ms: 5,
    },
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 4,
    created_by: "system",
    batch_id: "",
    concurrency_key: "email-send",
    created_at: offset(-300),
  },
  {
    id: "run_09",
    job_id: "job_sync_inventory",
    project_id: "proj_1",
    tags: { env: "staging" },
    status: "waiting",
    attempt: 1,
    payload: { warehouse: "eu-west-1" },
    result: null,
    metadata: {},
    error: "",
    triggered_by: "workflow",
    scheduled_at: offset(5),
    started_at: offset(6),
    finished_at: null,
    heartbeat_at: offset(12),
    next_retry_at: null,
    expires_at: offset(65),
    parent_run_id: "",
    priority: 6,
    idempotency_key: "sync-inv-staging-001",
    job_version: 2,
    job_version_id: "ver_inv_2",
    workflow_step_run_id: "wsr_inv_01",
    max_attempts_override: 0,
    timeout_secs_override: 0,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 5,
    retry_max_delay_secs: 5,
    execution_trace: null,
    debug_mode: true,
    continuation_of: "",
    lineage_depth: 1,
    created_by: "user_42",
    batch_id: "",
    concurrency_key: "",
    created_at: offset(5),
  },
  {
    id: "run_10",
    job_id: "job_generate_report",
    project_id: "proj_1",
    tags: { env: "production", report: "weekly" },
    status: "canceled",
    attempt: 1,
    payload: { week: "2026-W10", format: "csv" },
    result: null,
    metadata: {},
    error: "canceled by user",
    triggered_by: "manual",
    scheduled_at: offset(-30),
    started_at: offset(-29),
    finished_at: offset(-25),
    heartbeat_at: offset(-26),
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "",
    priority: 3,
    idempotency_key: "report-w10-csv",
    job_version: 1,
    job_version_id: "ver_report_1",
    workflow_step_run_id: "",
    max_attempts_override: 0,
    timeout_secs_override: 0,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 10,
    retry_max_delay_secs: 10,
    execution_trace: {
      queue_wait_ms: 40,
      dequeue_ms: 3,
      connect_ms: 12,
      ttfb_ms: 90,
      transfer_ms: 0,
      total_ms: 240_000,
      dispatch_ms: 7,
    },
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 0,
    created_by: "user_42",
    batch_id: "",
    concurrency_key: "",
    created_at: offset(-30),
  },
];

/** Generate 5 mock events for a given run. */
const mockEventsForRun = (runId: string): RunEvent[] => [
  {
    id: `${runId}_evt_1`,
    run_id: runId,
    type: "state_change",
    level: "info",
    message: "Run transitioned to executing",
    data: { from: "queued", to: "executing" },
    created_at: offset(1),
  },
  {
    id: `${runId}_evt_2`,
    run_id: runId,
    type: "log",
    level: "info",
    message: "Starting payload processing",
    data: null,
    created_at: offset(2),
  },
  {
    id: `${runId}_evt_3`,
    run_id: runId,
    type: "progress",
    level: "info",
    message: "Progress: 50% complete",
    data: { percent: 50 },
    created_at: offset(3),
  },
  {
    id: `${runId}_evt_4`,
    run_id: runId,
    type: "log",
    level: "debug",
    message: "Checkpoint saved",
    data: { checkpoint_id: "ckpt_001" },
    created_at: offset(4),
  },
  {
    id: `${runId}_evt_5`,
    run_id: runId,
    type: "state_change",
    level: "info",
    message: "Run transitioned to completed",
    data: { from: "executing", to: "completed" },
    created_at: offset(5),
  },
];

// ---------------------------------------------------------------------------
// Data access functions (mock)
// ---------------------------------------------------------------------------

// TODO: Replace with real API call
async function listRuns(
  params: RunsSearchParams
): Promise<PaginatedResponse<JobRun>> {
  await Promise.resolve();
  let filtered = MOCK_RUNS;

  if (params.query) {
    const q = params.query.toLowerCase();
    filtered = filtered.filter(
      (r) =>
        r.id.toLowerCase().includes(q) ||
        r.job_id.toLowerCase().includes(q) ||
        r.error.toLowerCase().includes(q)
    );
  }

  if (params.status && params.status.length > 0) {
    const set = new Set(params.status);
    filtered = filtered.filter((r) => set.has(r.status));
  }

  if (params.sort === "asc") {
    filtered = [...filtered].sort(
      (a, b) =>
        new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
    );
  } else {
    filtered = [...filtered].sort(
      (a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
    );
  }

  const page = params.page ?? 1;
  const perPage = params.per_page ?? 20;
  const start = (page - 1) * perPage;
  const paged = filtered.slice(start, start + perPage);

  return {
    data: paged,
    total_count: filtered.length,
    page_count: Math.ceil(filtered.length / perPage),
  };
}

// TODO: Replace with real API call
async function getRun(id: string): Promise<JobRun | null> {
  await Promise.resolve();
  return MOCK_RUNS.find((r) => r.id === id) ?? null;
}

// TODO: Replace with real API call
async function getRunEvents(runId: string): Promise<RunEvent[]> {
  await Promise.resolve();
  return mockEventsForRun(runId);
}

// TODO: Replace with real API call
async function retryRun(data: {
  run_id: string;
}): Promise<{ success: boolean }> {
  await Promise.resolve();
  const run = MOCK_RUNS.find((r) => r.id === data.run_id);
  if (!run) {
    throw new Error(`Run ${data.run_id} not found`);
  }
  return { success: true };
}

// TODO: Replace with real API call
async function cancelRun(data: {
  run_id: string;
}): Promise<{ success: boolean }> {
  await Promise.resolve();
  const run = MOCK_RUNS.find((r) => r.id === data.run_id);
  if (!run) {
    throw new Error(`Run ${data.run_id} not found`);
  }
  return { success: true };
}

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

export const runsQueryOptions = (search?: RunsSearchParams) =>
  queryOptions({
    queryKey: ["runs", search ?? {}],
    queryFn: () => listRuns(search ?? {}),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const runQueryOptions = (id: string) =>
  queryOptions({
    queryKey: ["runs", id],
    queryFn: () => getRun(id),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const runEventsQueryOptions = (runId: string) =>
  queryOptions({
    queryKey: ["runs", runId, "events"],
    queryFn: () => getRunEvents(runId),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export const useRetryRun = () =>
  useMutation({
    mutationKey: ["runs", "retry"],
    mutationFn: (data: { run_id: string }) => retryRun(data),
  });

export const useCancelRun = () =>
  useMutation({
    mutationKey: ["runs", "cancel"],
    mutationFn: (data: { run_id: string }) => cancelRun(data),
  });
