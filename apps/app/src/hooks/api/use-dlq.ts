import {
  keepPreviousData,
  queryOptions,
  useMutation,
} from "@tanstack/react-query";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils.ts";
import type { JobRun, ListParams } from "./types.ts";

// ---------------------------------------------------------------------------
// Mock data — dead letter queue items (runs with status "dead_letter")
// ---------------------------------------------------------------------------

const MOCK_DLQ_ITEMS: JobRun[] = [
  {
    id: "dlq_run_01",
    job_id: "job_payment_sync",
    project_id: "proj_01",
    tags: { team: "billing" },
    status: "dead_letter",
    attempt: 5,
    payload: { invoice_id: "inv_8837", amount_cents: 4999 },
    result: null,
    metadata: { correlation_id: "corr_aab1" },
    error: "Connection refused: payment gateway unreachable after 5 attempts",
    triggered_by: "cron",
    scheduled_at: "2026-03-10T02:00:00Z",
    started_at: "2026-03-10T02:00:03Z",
    finished_at: "2026-03-10T02:05:47Z",
    heartbeat_at: "2026-03-10T02:05:30Z",
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "",
    priority: 10,
    idempotency_key: "pay_sync_inv_8837",
    job_version: 2,
    job_version_id: "ver_ps_02",
    workflow_step_run_id: "",
    max_attempts_override: 0,
    timeout_secs_override: 0,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 5,
    retry_max_delay_secs: 300,
    execution_trace: {
      queue_wait_ms: 120,
      dequeue_ms: 8,
      connect_ms: 5002,
      ttfb_ms: 0,
      transfer_ms: 0,
      total_ms: 5130,
      dispatch_ms: 3,
    },
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 0,
    created_by: "system",
    batch_id: "",
    concurrency_key: "",
    created_at: "2026-03-10T02:00:00Z",
  },
  {
    id: "dlq_run_02",
    job_id: "job_email_send",
    project_id: "proj_01",
    tags: { team: "notifications" },
    status: "dead_letter",
    attempt: 3,
    payload: { to: "user@example.com", template: "welcome" },
    result: null,
    metadata: {},
    error: "SMTP 550: mailbox not found — permanent failure",
    triggered_by: "manual",
    scheduled_at: null,
    started_at: "2026-03-09T14:22:10Z",
    finished_at: "2026-03-09T14:22:11Z",
    heartbeat_at: null,
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "",
    priority: 5,
    idempotency_key: "",
    job_version: 1,
    job_version_id: "ver_es_01",
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
    created_by: "user_02",
    batch_id: "",
    concurrency_key: "",
    created_at: "2026-03-09T14:22:00Z",
  },
  {
    id: "dlq_run_03",
    job_id: "job_report_gen",
    project_id: "proj_01",
    tags: { team: "reporting" },
    status: "dead_letter",
    attempt: 4,
    payload: { report_type: "monthly_summary", tenant_id: "t_42" },
    result: null,
    metadata: { tenant: "acme-corp" },
    error: "OOM killed: container exceeded 2Gi memory limit during PDF render",
    triggered_by: "cron",
    scheduled_at: "2026-03-08T06:00:00Z",
    started_at: "2026-03-08T06:00:05Z",
    finished_at: "2026-03-08T06:12:33Z",
    heartbeat_at: "2026-03-08T06:12:00Z",
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "",
    priority: 5,
    idempotency_key: "report_monthly_t42_2026_02",
    job_version: 3,
    job_version_id: "ver_rg_03",
    workflow_step_run_id: "",
    max_attempts_override: 0,
    timeout_secs_override: 0,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 30,
    retry_max_delay_secs: 600,
    execution_trace: {
      queue_wait_ms: 200,
      dequeue_ms: 12,
      connect_ms: 45,
      ttfb_ms: 80,
      transfer_ms: 0,
      total_ms: 748_337,
      dispatch_ms: 5,
    },
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 0,
    created_by: "system",
    batch_id: "",
    concurrency_key: "report_gen",
    created_at: "2026-03-08T06:00:00Z",
  },
  {
    id: "dlq_run_04",
    job_id: "job_webhook_relay",
    project_id: "proj_01",
    tags: { team: "integrations" },
    status: "dead_letter",
    attempt: 5,
    payload: {
      event: "order.created",
      target: "https://partner.example.com/hooks",
    },
    result: null,
    metadata: { partner: "acme" },
    error: "TLS handshake timeout: remote certificate expired",
    triggered_by: "spawn",
    scheduled_at: null,
    started_at: "2026-03-07T19:45:00Z",
    finished_at: "2026-03-07T19:45:31Z",
    heartbeat_at: null,
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "run_parent_77",
    priority: 8,
    idempotency_key: "",
    job_version: 1,
    job_version_id: "ver_wr_01",
    workflow_step_run_id: "",
    max_attempts_override: 5,
    timeout_secs_override: 30,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 5,
    retry_max_delay_secs: 120,
    execution_trace: null,
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 1,
    created_by: "system",
    batch_id: "batch_wh_01",
    concurrency_key: "webhook_partner_acme",
    created_at: "2026-03-07T19:44:50Z",
  },
  {
    id: "dlq_run_05",
    job_id: "job_data_import",
    project_id: "proj_01",
    tags: { team: "data-eng" },
    status: "dead_letter",
    attempt: 3,
    payload: { source: "s3://bucket/imports/2026-03-06.csv", format: "csv" },
    result: null,
    metadata: {},
    error:
      "Schema validation failed: column 'amount' expected numeric, got string at row 14832",
    triggered_by: "manual",
    scheduled_at: null,
    started_at: "2026-03-06T11:30:00Z",
    finished_at: "2026-03-06T11:30:42Z",
    heartbeat_at: "2026-03-06T11:30:40Z",
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "",
    priority: 5,
    idempotency_key: "import_2026_03_06",
    job_version: 2,
    job_version_id: "ver_di_02",
    workflow_step_run_id: "",
    max_attempts_override: 0,
    timeout_secs_override: 0,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 60,
    retry_max_delay_secs: 60,
    execution_trace: {
      queue_wait_ms: 50,
      dequeue_ms: 6,
      connect_ms: 22,
      ttfb_ms: 110,
      transfer_ms: 38_000,
      total_ms: 42_188,
      dispatch_ms: 4,
    },
    debug_mode: true,
    continuation_of: "",
    lineage_depth: 0,
    created_by: "user_03",
    batch_id: "",
    concurrency_key: "",
    created_at: "2026-03-06T11:29:50Z",
  },
  {
    id: "dlq_run_06",
    job_id: "job_cache_invalidation",
    project_id: "proj_01",
    tags: { team: "platform" },
    status: "dead_letter",
    attempt: 3,
    payload: { keys: ["user:*", "session:*"], region: "us-east-1" },
    result: null,
    metadata: {},
    error: "Redis cluster: CLUSTERDOWN The cluster is down",
    triggered_by: "workflow",
    scheduled_at: null,
    started_at: "2026-03-05T08:10:00Z",
    finished_at: "2026-03-05T08:10:05Z",
    heartbeat_at: null,
    next_retry_at: null,
    expires_at: null,
    parent_run_id: "",
    priority: 10,
    idempotency_key: "",
    job_version: 1,
    job_version_id: "ver_ci_01",
    workflow_step_run_id: "wsr_deploy_03",
    max_attempts_override: 0,
    timeout_secs_override: 10,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 5,
    retry_max_delay_secs: 5,
    execution_trace: null,
    debug_mode: false,
    continuation_of: "",
    lineage_depth: 2,
    created_by: "system",
    batch_id: "",
    concurrency_key: "",
    created_at: "2026-03-05T08:09:55Z",
  },
  {
    id: "dlq_run_07",
    job_id: "job_user_cleanup",
    project_id: "proj_01",
    tags: { team: "identity" },
    status: "dead_letter",
    attempt: 5,
    payload: {
      user_ids: ["u_deleted_01", "u_deleted_02"],
      action: "purge_pii",
    },
    result: null,
    metadata: { gdpr_request: "req_991" },
    error:
      "Deadlock detected: concurrent PII purge on shared partition; transaction rolled back",
    triggered_by: "retry",
    scheduled_at: null,
    started_at: "2026-03-04T23:00:00Z",
    finished_at: "2026-03-04T23:00:18Z",
    heartbeat_at: "2026-03-04T23:00:15Z",
    next_retry_at: null,
    expires_at: "2026-03-11T23:00:00Z",
    parent_run_id: "",
    priority: 10,
    idempotency_key: "gdpr_purge_req_991",
    job_version: 4,
    job_version_id: "ver_uc_04",
    workflow_step_run_id: "",
    max_attempts_override: 5,
    timeout_secs_override: 60,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 10,
    retry_max_delay_secs: 300,
    execution_trace: {
      queue_wait_ms: 80,
      dequeue_ms: 5,
      connect_ms: 18,
      ttfb_ms: 42,
      transfer_ms: 0,
      total_ms: 18_145,
      dispatch_ms: 3,
    },
    debug_mode: false,
    continuation_of: "dlq_run_07_prev",
    lineage_depth: 1,
    created_by: "system",
    batch_id: "",
    concurrency_key: "pii_purge",
    created_at: "2026-03-04T22:59:50Z",
  },
];

// ---------------------------------------------------------------------------
// Data-access functions (mock)
// ---------------------------------------------------------------------------

// TODO: Replace with real API call
async function listDlq(params: ListParams) {
  await Promise.resolve();
  let items = [...MOCK_DLQ_ITEMS];

  if (params.query) {
    const q = params.query.toLowerCase();
    items = items.filter(
      (r) =>
        r.job_id.toLowerCase().includes(q) ||
        r.error.toLowerCase().includes(q) ||
        r.id.toLowerCase().includes(q)
    );
  }

  if (params.sort === "desc") {
    items.reverse();
  }

  const page = params.page ?? 1;
  const perPage = params.per_page ?? 20;
  const start = (page - 1) * perPage;
  const paged = items.slice(start, start + perPage);

  return {
    data: paged,
    total_count: items.length,
    page_count: Math.ceil(items.length / perPage),
  };
}

// TODO: Replace with real API call
async function retryDlqItem(data: { id: string }) {
  await Promise.resolve();
  const item = MOCK_DLQ_ITEMS.find((r) => r.id === data.id);
  if (!item) {
    throw new Error(`DLQ item ${data.id} not found`);
  }
  return { ...item, status: "queued" as const };
}

// TODO: Replace with real API call
async function discardDlqItem(data: { id: string }) {
  await Promise.resolve();
  const item = MOCK_DLQ_ITEMS.find((r) => r.id === data.id);
  if (!item) {
    throw new Error(`DLQ item ${data.id} not found`);
  }
  return { success: true, id: data.id };
}

// TODO: Replace with real API call
async function bulkRetryDlq(data: { ids: string[] }) {
  await Promise.resolve();
  return { retried: data.ids.length, ids: data.ids };
}

// TODO: Replace with real API call
async function bulkDiscardDlq(data: { ids: string[] }) {
  await Promise.resolve();
  return { discarded: data.ids.length, ids: data.ids };
}

// ---------------------------------------------------------------------------
// Query options & mutations
// ---------------------------------------------------------------------------

/** Query options for listing dead letter queue items. */
export const dlqQueryOptions = (search?: ListParams) =>
  queryOptions({
    queryKey: ["dlq", search],
    queryFn: () => listDlq(search ?? {}),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

/** Retries a single DLQ item by id. */
export const useRetryDlqItem = () =>
  useMutation({
    mutationKey: ["dlq", "retry"],
    mutationFn: (data: { id: string }) => retryDlqItem(data),
  });

/** Discards a single DLQ item by id. */
export const useDiscardDlqItem = () =>
  useMutation({
    mutationKey: ["dlq", "discard"],
    mutationFn: (data: { id: string }) => discardDlqItem(data),
  });

/** Retries multiple DLQ items in bulk. */
export const useBulkRetryDlq = () =>
  useMutation({
    mutationKey: ["dlq", "bulkRetry"],
    mutationFn: (data: { ids: string[] }) => bulkRetryDlq(data),
  });

/** Discards multiple DLQ items in bulk. */
export const useBulkDiscardDlq = () =>
  useMutation({
    mutationKey: ["dlq", "bulkDiscard"],
    mutationFn: (data: { ids: string[] }) => bulkDiscardDlq(data),
  });
