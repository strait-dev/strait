import {
  keepPreviousData,
  queryOptions,
  useMutation,
} from "@tanstack/react-query";
import type { Job, ListParams, PaginatedResponse } from "@/hooks/api/types.ts";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils.ts";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const now = new Date().toISOString();
const yesterday = new Date(Date.now() - 86_400_000).toISOString();
const twoDaysAgo = new Date(Date.now() - 2 * 86_400_000).toISOString();

const MOCK_JOBS: Job[] = [
  {
    id: "job_01",
    project_id: "proj_01",
    group_id: "grp_payments",
    name: "Process Payments",
    slug: "process-payments",
    description: "Processes pending payment transactions in batch",
    cron: "*/15 * * * *",
    payload_schema: null,
    tags: { team: "billing", priority: "high" },
    endpoint_url: "https://api.example.com/jobs/process-payments",
    fallback_endpoint_url: "",
    max_attempts: 3,
    timeout_secs: 120,
    max_concurrency: 5,
    max_concurrency_per_key: 1,
    execution_window_cron: "",
    timezone: "UTC",
    rate_limit_max: 100,
    rate_limit_window_secs: 60,
    rate_limit_keys: [],
    dedup_window_secs: 300,
    enabled: true,
    webhook_url: "",
    webhook_secret: "",
    run_ttl_secs: 86_400,
    retry_strategy: "exponential",
    retry_delays_secs: [5, 30, 120],
    environment_id: "env_prod",
    default_run_metadata: {},
    version: 3,
    version_id: "ver_pay_03",
    version_policy: "latest",
    backwards_compatible: true,
    created_by: "user_01",
    updated_by: "user_01",
    created_at: twoDaysAgo,
    updated_at: now,
  },
  {
    id: "job_02",
    project_id: "proj_01",
    group_id: "grp_notifications",
    name: "Send Email Batch",
    slug: "send-email-batch",
    description: "Sends queued transactional and marketing emails",
    cron: "0 */2 * * *",
    payload_schema: null,
    tags: { team: "comms", priority: "medium" },
    endpoint_url: "https://api.example.com/jobs/send-email-batch",
    fallback_endpoint_url: "",
    max_attempts: 5,
    timeout_secs: 300,
    max_concurrency: 10,
    max_concurrency_per_key: 2,
    execution_window_cron: "",
    timezone: "UTC",
    rate_limit_max: 500,
    rate_limit_window_secs: 60,
    rate_limit_keys: [],
    dedup_window_secs: 0,
    enabled: true,
    webhook_url: "",
    webhook_secret: "",
    run_ttl_secs: 43_200,
    retry_strategy: "exponential",
    retry_delays_secs: [10, 60, 300],
    environment_id: "env_prod",
    default_run_metadata: {},
    version: 1,
    version_id: "ver_email_01",
    version_policy: "latest",
    backwards_compatible: true,
    created_by: "user_01",
    updated_by: "user_01",
    created_at: twoDaysAgo,
    updated_at: yesterday,
  },
  {
    id: "job_03",
    project_id: "proj_01",
    group_id: "grp_inventory",
    name: "Sync Inventory",
    slug: "sync-inventory",
    description:
      "Synchronizes inventory levels with external warehouse systems",
    cron: "0 * * * *",
    payload_schema: null,
    tags: { team: "logistics", priority: "high" },
    endpoint_url: "https://api.example.com/jobs/sync-inventory",
    fallback_endpoint_url: "https://fallback.example.com/jobs/sync-inventory",
    max_attempts: 3,
    timeout_secs: 180,
    max_concurrency: 3,
    max_concurrency_per_key: 1,
    execution_window_cron: "",
    timezone: "America/New_York",
    rate_limit_max: 50,
    rate_limit_window_secs: 60,
    rate_limit_keys: [],
    dedup_window_secs: 600,
    enabled: true,
    webhook_url: "",
    webhook_secret: "",
    run_ttl_secs: 86_400,
    retry_strategy: "fixed",
    retry_delays_secs: [30, 30, 30],
    environment_id: "env_prod",
    default_run_metadata: {},
    version: 2,
    version_id: "ver_inv_02",
    version_policy: "minor",
    backwards_compatible: true,
    created_by: "user_02",
    updated_by: "user_02",
    created_at: twoDaysAgo,
    updated_at: now,
  },
  {
    id: "job_04",
    project_id: "proj_01",
    group_id: "grp_reporting",
    name: "Generate Report",
    slug: "generate-report",
    description: "Generates daily analytics and financial reports",
    cron: "0 2 * * *",
    payload_schema: null,
    tags: { team: "analytics", priority: "low" },
    endpoint_url: "https://api.example.com/jobs/generate-report",
    fallback_endpoint_url: "",
    max_attempts: 2,
    timeout_secs: 600,
    max_concurrency: 1,
    max_concurrency_per_key: 1,
    execution_window_cron: "",
    timezone: "UTC",
    rate_limit_max: 0,
    rate_limit_window_secs: 0,
    rate_limit_keys: [],
    dedup_window_secs: 3600,
    enabled: true,
    webhook_url: "",
    webhook_secret: "",
    run_ttl_secs: 172_800,
    retry_strategy: "exponential",
    retry_delays_secs: [60, 300],
    environment_id: "env_prod",
    default_run_metadata: {},
    version: 5,
    version_id: "ver_rpt_05",
    version_policy: "pin",
    backwards_compatible: false,
    created_by: "user_01",
    updated_by: "user_03",
    created_at: twoDaysAgo,
    updated_at: yesterday,
  },
  {
    id: "job_05",
    project_id: "proj_01",
    group_id: "grp_infra",
    name: "Backup Database",
    slug: "backup-database",
    description: "Creates incremental backups of the primary database",
    cron: "0 3 * * *",
    payload_schema: null,
    tags: { team: "infra", priority: "critical" },
    endpoint_url: "https://api.example.com/jobs/backup-database",
    fallback_endpoint_url: "",
    max_attempts: 3,
    timeout_secs: 900,
    max_concurrency: 1,
    max_concurrency_per_key: 1,
    execution_window_cron: "",
    timezone: "UTC",
    rate_limit_max: 0,
    rate_limit_window_secs: 0,
    rate_limit_keys: [],
    dedup_window_secs: 7200,
    enabled: true,
    webhook_url: "https://hooks.example.com/backup-status",
    webhook_secret: "whsec_backup_01",
    run_ttl_secs: 259_200,
    retry_strategy: "exponential",
    retry_delays_secs: [30, 120, 600],
    environment_id: "env_prod",
    default_run_metadata: {},
    version: 1,
    version_id: "ver_bkp_01",
    version_policy: "latest",
    backwards_compatible: true,
    created_by: "user_02",
    updated_by: "user_02",
    created_at: twoDaysAgo,
    updated_at: twoDaysAgo,
  },
  {
    id: "job_06",
    project_id: "proj_01",
    group_id: "grp_infra",
    name: "Cleanup Temp Files",
    slug: "cleanup-temp-files",
    description: "Removes expired temporary files and stale upload artifacts",
    cron: "0 4 * * *",
    payload_schema: null,
    tags: { team: "infra", priority: "low" },
    endpoint_url: "https://api.example.com/jobs/cleanup-temp-files",
    fallback_endpoint_url: "",
    max_attempts: 1,
    timeout_secs: 60,
    max_concurrency: 1,
    max_concurrency_per_key: 1,
    execution_window_cron: "",
    timezone: "UTC",
    rate_limit_max: 0,
    rate_limit_window_secs: 0,
    rate_limit_keys: [],
    dedup_window_secs: 0,
    enabled: false,
    webhook_url: "",
    webhook_secret: "",
    run_ttl_secs: 43_200,
    retry_strategy: "fixed",
    retry_delays_secs: [],
    environment_id: "env_prod",
    default_run_metadata: {},
    version: 1,
    version_id: "ver_tmp_01",
    version_policy: "latest",
    backwards_compatible: true,
    created_by: "user_03",
    updated_by: "user_03",
    created_at: yesterday,
    updated_at: yesterday,
  },
];

// ---------------------------------------------------------------------------
// Data access functions
// ---------------------------------------------------------------------------

type ListJobsInput = ListParams & { status?: string[] };

async function listJobs(data: ListJobsInput): Promise<PaginatedResponse<Job>> {
  // TODO: Replace with real API call
  await Promise.resolve();
  let filtered = [...MOCK_JOBS];

  if (data.query) {
    const q = data.query.toLowerCase();
    filtered = filtered.filter(
      (j) =>
        j.name.toLowerCase().includes(q) ||
        j.slug.toLowerCase().includes(q) ||
        j.description.toLowerCase().includes(q)
    );
  }

  if (data.status && data.status.length > 0) {
    const enabledSet = new Set(data.status.map((s) => s === "enabled"));
    filtered = filtered.filter((j) => enabledSet.has(j.enabled));
  }

  if (data.sort === "desc") {
    filtered.reverse();
  }

  const page = data.page ?? 1;
  const perPage = data.per_page ?? 20;
  const start = (page - 1) * perPage;
  const paged = filtered.slice(start, start + perPage);

  return {
    data: paged,
    page_count: Math.ceil(filtered.length / perPage),
    total_count: filtered.length,
  };
}

async function getJob(data: { id: string }) {
  // TODO: Replace with real API call
  await Promise.resolve();
  return MOCK_JOBS.find((j) => j.id === data.id) ?? null;
}

async function triggerJob(data: { id: string; payload?: unknown }) {
  // TODO: Replace with real API call
  await Promise.resolve();
  const job = MOCK_JOBS.find((j) => j.id === data.id);
  if (!job) {
    throw new Error(`Job not found: ${data.id}`);
  }
  return { success: true, job_id: job.id };
}

async function pauseJob(data: { id: string }) {
  // TODO: Replace with real API call
  await Promise.resolve();
  const job = MOCK_JOBS.find((j) => j.id === data.id);
  if (!job) {
    throw new Error(`Job not found: ${data.id}`);
  }
  return { success: true, job_id: job.id };
}

async function resumeJob(data: { id: string }) {
  // TODO: Replace with real API call
  await Promise.resolve();
  const job = MOCK_JOBS.find((j) => j.id === data.id);
  if (!job) {
    throw new Error(`Job not found: ${data.id}`);
  }
  return { success: true, job_id: job.id };
}

async function deleteJob(data: { id: string }) {
  // TODO: Replace with real API call
  await Promise.resolve();
  const job = MOCK_JOBS.find((j) => j.id === data.id);
  if (!job) {
    throw new Error(`Job not found: ${data.id}`);
  }
  return { success: true, job_id: job.id };
}

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

export const jobsQueryOptions = (search?: ListJobsInput) =>
  queryOptions({
    queryKey: ["jobs", search ?? {}],
    queryFn: () => listJobs(search ?? {}),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const jobQueryOptions = (id: string) =>
  queryOptions({
    queryKey: ["jobs", id],
    queryFn: () => getJob({ id }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

// ---------------------------------------------------------------------------
// Mutation hooks
// ---------------------------------------------------------------------------

export const useTriggerJob = () =>
  useMutation({
    mutationKey: ["jobs", "trigger"],
    mutationFn: (data: { id: string; payload?: unknown }) => triggerJob(data),
  });

export const usePauseJob = () =>
  useMutation({
    mutationKey: ["jobs", "pause"],
    mutationFn: (data: { id: string }) => pauseJob(data),
  });

export const useResumeJob = () =>
  useMutation({
    mutationKey: ["jobs", "resume"],
    mutationFn: (data: { id: string }) => resumeJob(data),
  });

export const useDeleteJob = () =>
  useMutation({
    mutationKey: ["jobs", "delete"],
    mutationFn: (data: { id: string }) => deleteJob(data),
  });
