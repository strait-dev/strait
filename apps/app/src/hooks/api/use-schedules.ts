import {
  keepPreviousData,
  queryOptions,
  useMutation,
} from "@tanstack/react-query";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import type { Job, ListParams } from "./types";

const MOCK_SCHEDULES: Job[] = [
  {
    id: "sched_01",
    project_id: "proj_01",
    group_id: "grp_01",
    name: "Daily billing reconciliation",
    slug: "daily-billing-reconciliation",
    description:
      "Reconciles billing records against payment gateway every day at 2 AM UTC.",
    cron: "0 2 * * *",
    payload_schema: null,
    tags: { team: "billing" },
    endpoint_url: "https://api.example.com/jobs/billing-reconciliation",
    fallback_endpoint_url: "",
    max_attempts: 3,
    timeout_secs: 300,
    max_concurrency: 1,
    max_concurrency_per_key: 0,
    execution_window_cron: "",
    timezone: "UTC",
    rate_limit_max: 0,
    rate_limit_window_secs: 0,
    rate_limit_keys: [],
    dedup_window_secs: 0,
    enabled: true,
    webhook_url: "",
    webhook_secret: "",
    run_ttl_secs: 86_400,
    retry_strategy: "exponential",
    retry_delays_secs: [5, 30, 120],
    environment_id: "env_prod",
    default_run_metadata: {},
    version: 3,
    version_id: "ver_01",
    version_policy: "latest",
    backwards_compatible: true,
    created_by: "user_01",
    updated_by: "user_01",
    created_at: "2025-11-01T10:00:00Z",
    updated_at: "2026-01-15T08:30:00Z",
  },
  {
    id: "sched_02",
    project_id: "proj_01",
    group_id: "grp_02",
    name: "Hourly metrics aggregation",
    slug: "hourly-metrics-aggregation",
    description: "Aggregates raw metrics into rollup tables every hour.",
    cron: "0 * * * *",
    payload_schema: null,
    tags: { team: "platform" },
    endpoint_url: "https://api.example.com/jobs/metrics-aggregation",
    fallback_endpoint_url: "",
    max_attempts: 2,
    timeout_secs: 600,
    max_concurrency: 2,
    max_concurrency_per_key: 0,
    execution_window_cron: "",
    timezone: "UTC",
    rate_limit_max: 0,
    rate_limit_window_secs: 0,
    rate_limit_keys: [],
    dedup_window_secs: 0,
    enabled: true,
    webhook_url: "",
    webhook_secret: "",
    run_ttl_secs: 3600,
    retry_strategy: "fixed",
    retry_delays_secs: [10],
    environment_id: "env_prod",
    default_run_metadata: {},
    version: 1,
    version_id: "ver_02",
    version_policy: "latest",
    backwards_compatible: true,
    created_by: "user_01",
    updated_by: "user_01",
    created_at: "2025-12-05T14:00:00Z",
    updated_at: "2026-02-10T09:00:00Z",
  },
  {
    id: "sched_03",
    project_id: "proj_01",
    group_id: "grp_01",
    name: "Weekly report generation",
    slug: "weekly-report-generation",
    description: "Generates PDF reports for all tenants every Monday at 6 AM.",
    cron: "0 6 * * 1",
    payload_schema: null,
    tags: { team: "reporting" },
    endpoint_url: "https://api.example.com/jobs/weekly-reports",
    fallback_endpoint_url: "https://fallback.example.com/jobs/weekly-reports",
    max_attempts: 5,
    timeout_secs: 1800,
    max_concurrency: 4,
    max_concurrency_per_key: 1,
    execution_window_cron: "",
    timezone: "America/New_York",
    rate_limit_max: 0,
    rate_limit_window_secs: 0,
    rate_limit_keys: [],
    dedup_window_secs: 0,
    enabled: true,
    webhook_url: "https://hooks.example.com/report-done",
    webhook_secret: "whsec_report",
    run_ttl_secs: 604_800,
    retry_strategy: "exponential",
    retry_delays_secs: [10, 60, 300],
    environment_id: "env_prod",
    default_run_metadata: {},
    version: 2,
    version_id: "ver_03",
    version_policy: "pin",
    backwards_compatible: false,
    created_by: "user_02",
    updated_by: "user_02",
    created_at: "2025-10-20T12:00:00Z",
    updated_at: "2026-01-08T15:45:00Z",
  },
  {
    id: "sched_04",
    project_id: "proj_01",
    group_id: "grp_03",
    name: "Nightly cache warm-up",
    slug: "nightly-cache-warmup",
    description:
      "Pre-warms CDN and application caches every night at midnight.",
    cron: "0 0 * * *",
    payload_schema: null,
    tags: { team: "platform", priority: "low" },
    endpoint_url: "https://api.example.com/jobs/cache-warmup",
    fallback_endpoint_url: "",
    max_attempts: 2,
    timeout_secs: 900,
    max_concurrency: 1,
    max_concurrency_per_key: 0,
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
    retry_delays_secs: [30],
    environment_id: "env_prod",
    default_run_metadata: {},
    version: 1,
    version_id: "ver_04",
    version_policy: "latest",
    backwards_compatible: true,
    created_by: "user_01",
    updated_by: "user_01",
    created_at: "2026-01-10T08:00:00Z",
    updated_at: "2026-03-01T11:20:00Z",
  },
  {
    id: "sched_05",
    project_id: "proj_01",
    group_id: "grp_02",
    name: "Every-15-min health check",
    slug: "health-check-15m",
    description:
      "Pings all registered endpoints and records latency every 15 minutes.",
    cron: "*/15 * * * *",
    payload_schema: null,
    tags: { team: "sre" },
    endpoint_url: "https://api.example.com/jobs/health-check",
    fallback_endpoint_url: "",
    max_attempts: 1,
    timeout_secs: 30,
    max_concurrency: 1,
    max_concurrency_per_key: 0,
    execution_window_cron: "",
    timezone: "UTC",
    rate_limit_max: 0,
    rate_limit_window_secs: 0,
    rate_limit_keys: [],
    dedup_window_secs: 0,
    enabled: true,
    webhook_url: "",
    webhook_secret: "",
    run_ttl_secs: 3600,
    retry_strategy: "fixed",
    retry_delays_secs: [],
    environment_id: "env_prod",
    default_run_metadata: {},
    version: 5,
    version_id: "ver_05",
    version_policy: "latest",
    backwards_compatible: true,
    created_by: "user_03",
    updated_by: "user_03",
    created_at: "2025-09-15T16:00:00Z",
    updated_at: "2026-03-10T07:00:00Z",
  },
];

// TODO: Replace with real API call
async function listSchedules(params: ListParams) {
  await Promise.resolve();
  let items = [...MOCK_SCHEDULES];

  if (params.query) {
    const q = params.query.toLowerCase();
    items = items.filter(
      (s) =>
        s.name.toLowerCase().includes(q) ||
        s.slug.toLowerCase().includes(q) ||
        s.cron.includes(q)
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
async function pauseSchedule(data: { id: string }) {
  await Promise.resolve();
  const schedule = MOCK_SCHEDULES.find((s) => s.id === data.id);
  if (!schedule) {
    throw new Error(`Schedule ${data.id} not found`);
  }
  return { ...schedule, enabled: false };
}

// TODO: Replace with real API call
async function resumeSchedule(data: { id: string }) {
  await Promise.resolve();
  const schedule = MOCK_SCHEDULES.find((s) => s.id === data.id);
  if (!schedule) {
    throw new Error(`Schedule ${data.id} not found`);
  }
  return { ...schedule, enabled: true };
}

// TODO: Replace with real API call
async function triggerSchedule(data: { id: string }) {
  await Promise.resolve();
  const schedule = MOCK_SCHEDULES.find((s) => s.id === data.id);
  if (!schedule) {
    throw new Error(`Schedule ${data.id} not found`);
  }
  return { success: true, schedule_id: schedule.id };
}

/** Query options for listing schedules (cron-enabled jobs). */
export const schedulesQueryOptions = (search?: ListParams) =>
  queryOptions({
    queryKey: ["schedules", search],
    queryFn: () => listSchedules(search ?? {}),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

/** Pauses a schedule by id. */
export const usePauseSchedule = () =>
  useMutation({
    mutationKey: ["schedules", "pause"],
    mutationFn: (data: { id: string }) => pauseSchedule(data),
  });

/** Resumes a schedule by id. */
export const useResumeSchedule = () =>
  useMutation({
    mutationKey: ["schedules", "resume"],
    mutationFn: (data: { id: string }) => resumeSchedule(data),
  });

/** Triggers an immediate run for a schedule. */
export const useTriggerSchedule = () =>
  useMutation({
    mutationKey: ["schedules", "trigger"],
    mutationFn: (data: { id: string }) => triggerSchedule(data),
  });
