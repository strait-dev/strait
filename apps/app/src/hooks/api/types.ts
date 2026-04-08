// Canonical frontend types for the orchestration data model.
// Run `bun run generate:api` to regenerate the schema from the live Go API.

import type { components } from "@/lib/api/schema";

type Schema = components["schemas"];

/** Matches Go domain.RunStatus constants. */
export type RunStatus =
  | "delayed"
  | "queued"
  | "dequeued"
  | "executing"
  | "waiting"
  | "completed"
  | "failed"
  | "timed_out"
  | "crashed"
  | "system_failed"
  | "canceled"
  | "expired"
  | "dead_letter";

/** Matches Go domain.WorkflowRunStatus constants. */
export type WorkflowRunStatus =
  | "pending"
  | "running"
  | "paused"
  | "completed"
  | "failed"
  | "timed_out"
  | "canceled";

/** Matches Go domain.WorkflowStepType constants. */
export type WorkflowStepType =
  | "job"
  | "approval"
  | "sub_workflow"
  | "wait_for_event"
  | "sleep";

/** Matches Go domain.StepRunStatus constants. */
export type StepRunStatus =
  | "pending"
  | "waiting"
  | "running"
  | "completed"
  | "failed"
  | "skipped"
  | "canceled";

/** Matches Go domain.VersionPolicy. */
export type VersionPolicy = "pin" | "latest" | "minor";

/** Timing breakdown for a job run execution. */
export type ExecutionTrace = Schema["ExecutionTrace"];

/** Job definition. */
export type Job = Schema["Job"];

/** Job run. */
export type JobRun = Schema["JobRun"];

/** Workflow step (node in a workflow DAG). */
export type WorkflowStep = Schema["WorkflowStep"];

/** Workflow run. */
export type WorkflowRun = Schema["WorkflowRun"];

/** Webhook subscription. */
export type WebhookSubscription = Schema["WebhookSubscription"];

/** Webhook delivery. */
export type WebhookDelivery = Schema["WebhookDelivery"];

/** Run event. */
export type RunEvent = Schema["RunEvent"];

/** Event trigger (durable wait). */
export type EventTrigger = Schema["EventTrigger"];

/** Region metadata from GET /v1/regions. */
export type Region = Schema["RegionResponse"];

/** API key (create response includes the key field). */
export type APIKey = Schema["CreateAPIKeyResponse"];

/** Response from POST /v1/api-keys/{keyID}/rotate. */
export type RotateAPIKeyResponse = Schema["RotateAPIKeyRequest"];

/** Workflow DAG definition. Extracted from WorkflowResponse. */
export type Workflow = {
  id: string;
  project_id: string;
  name: string;
  slug: string;
  description: string;
  tags: Record<string, string>;
  enabled: boolean;
  version: number;
  timeout_secs: number;
  max_concurrent_runs: number;
  max_parallel_steps: number;
  cron: string;
  cron_timezone: string;
  skip_if_running: boolean;
  version_id: string;
  version_policy: VersionPolicy;
  backwards_compatible: boolean;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
};

/** Union of RunStatus and WorkflowRunStatus, used by StatusBadge. */
export type DisplayStatus = RunStatus | WorkflowRunStatus;

/** Cursor-based paginated response matching the Go API envelope. */
export type PaginatedResponse<T> = {
  data: T[];
  next_cursor?: string;
  has_more: boolean;
};

/** Common search/filter params for list endpoints (cursor-based). */
export type ListParams = {
  limit?: number;
  cursor?: string;
};

/** Job health stats from GET /v1/jobs/:id/health. */
export type JobHealthResponse = {
  job_id: string;
  window: string;
  since: string;
  total_runs: number;
  completed_runs: number;
  failed_runs: number;
  timed_out_runs: number;
  crashed_runs: number;
  canceled_runs: number;
  expired_runs: number;
  success_rate: number;
  avg_duration_secs: number;
  p95_duration_secs: number;
  p99_duration_secs: number;
  health_score: number;
};

/** Project settings from GET /v1/projects/:id/settings. */
export type ProjectSettings = Schema["ProjectSettingsResponse"];

/** Frontend-managed project entity (stored in the auth DB). */
export type Project = {
  id: string;
  organization_id: string;
  name: string;
  slug: string;
  description: string;
  created_by: string;
  created_at: string;
  updated_at: string;
};

/** Queue stats from GET /v1/stats. */
export type QueueStatsResponse = {
  queued: number;
  executing: number;
  delayed: number;
};

/** Individual job performance metrics from analytics. */
export type JobPerformance = {
  job_id: string;
  job_slug: string;
  avg_duration_secs: number;
  p95_duration_secs: number;
  total_runs: number;
  failed_runs: number;
};

/** Run throughput broken down by status. */
export type ThroughputStats = {
  completed: number;
  failed: number;
  timed_out: number;
  canceled: number;
  period_hours: number;
};

/** Overall health summary from analytics. */
export type HealthSummary = {
  total_jobs: number;
  active_jobs: number;
  success_rate: number;
  avg_duration_secs: number;
  queue_depth: number;
};

/** Performance analytics from GET /v1/analytics/performance. */
export type PerformanceAnalytics = {
  slowest_jobs: JobPerformance[];
  throughput: ThroughputStats;
  health_summary: HealthSummary;
};

export type NotifyDigestPolicy = "instant" | "hourly" | "daily";

export type NotifyCategoryType = "product" | "transactional" | "critical";

export type NotifySubscriberStatus = "active" | "unsubscribed" | "deleted";

export type NotifyDeliveryChannel = "email" | "inbox";

export type NotifyMessageStatus =
  | "rendering"
  | "scheduled"
  | "pending"
  | "processing"
  | "delivered"
  | "failed"
  | "bounced"
  | "cancelled";

export type NotifySubscriber = {
  id: string;
  project_id: string;
  external_id: string;
  email?: string;
  phone?: string;
  locale: string;
  timezone: string;
  push_tokens?: Record<string, object>;
  attributes?: Record<string, object>;
  tenant_id?: string;
  status: NotifySubscriberStatus;
  created_at: string;
  updated_at: string;
};

export type NotifyTopic = {
  id: string;
  project_id: string;
  topic_key: string;
  name: string;
  description?: string;
  attributes?: Record<string, object>;
  created_at: string;
};

export type NotificationTemplate = {
  id: string;
  project_id: string;
  template_key: string;
  name: string;
  description?: string;
  version: number;
  channels: Record<string, object>;
  variables?: Record<string, object>;
  locale_templates?: Record<string, object>;
  default_locale: string;
  status: string;
  created_at: string;
  updated_at: string;
};

export type NotificationCategory = {
  id: string;
  project_id: string;
  category_key: string;
  name: string;
  description?: string;
  type: NotifyCategoryType;
  created_at: string;
};

export type NotifyPolicyOverride = {
  id: string;
  project_id: string;
  scope_type: "project" | "category" | "workflow_step";
  scope_key: string;
  channel?: NotifyDeliveryChannel;
  digest_policy?: NotifyDigestPolicy;
  retry_max_attempts?: number | null;
  retry_base_delay_secs?: number | null;
  retry_max_delay_secs?: number | null;
  escalation_tiers?: number | null;
  escalation_min_interval_secs?: number | null;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

export type NotificationProvider = {
  id: string;
  project_id: string;
  channel: NotifyDeliveryChannel;
  provider: string;
  name: string;
  config?: Record<string, object>;
  is_default: boolean;
  fallback_id?: string;
  health: string;
  rate_limit?: number | null;
  created_at: string;
  updated_at: string;
};

export type NotifySuppressionEvent = {
  id: string;
  project_id: string;
  recipient_type: string;
  recipient_id: string;
  scope: string;
  channel: NotifyDeliveryChannel;
  action: "suppressed" | "unsuppressed";
  reason?: string;
  source: string;
  metadata?: Record<string, object>;
  created_at: string;
};

export type NotifyPreference = {
  id: string;
  recipient_type: string;
  recipient_id: string;
  scope: string;
  channel_prefs?: Record<string, object | string | number | boolean | null>;
  quiet_hours?: Record<string, object | string | number | boolean | null>;
  phone?: string;
  timezone?: string;
  digest_policy?: NotifyDigestPolicy;
  critical_override: boolean;
  rate_limit_override?: number | null;
  created_at: string;
  updated_at: string;
};

export type NotificationMessage = {
  id: string;
  project_id: string;
  recipient_type: string;
  recipient_id: string;
  tenant_id?: string;
  workflow_run_id?: string;
  step_run_id?: string;
  template_id?: string;
  category_key?: string;
  channel: NotifyDeliveryChannel;
  provider_id?: string;
  rendered_content?: Record<string, object>;
  status: NotifyMessageStatus;
  attempts: number;
  provider_response?: Record<string, object>;
  delivered_at?: string;
  read_at?: string;
  clicked_at?: string;
  bounced_at?: string;
  suppression_reason?: string;
  batch_id?: string;
  scheduled_at?: string;
  created_at: string;
};

export type NotifyEscalationState = {
  id: string;
  project_id: string;
  step_run_id: string;
  workflow_run_id: string;
  current_tier: number;
  total_tiers: number;
  acknowledged: boolean;
  acknowledged_by?: string;
  acknowledged_at?: string;
  next_escalation_at?: string;
  status: string;
  created_at: string;
  updated_at: string;
};

export type NotifyTriggerResult = {
  recipient_id: string;
  message_ids: string[];
};

export type NotifyTriggerResponse = {
  results: NotifyTriggerResult[];
};
