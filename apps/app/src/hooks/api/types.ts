// Canonical frontend types for the orchestration data model.
// Entity types are derived from the Huma-generated OpenAPI schema where available.
// Run `bun run generate:api` to regenerate the schema from the live Go API.

import type { components } from "@/lib/api/schema";

// ---------------------------------------------------------------------------
// Schema type alias for convenience
// ---------------------------------------------------------------------------

type Schema = components["schemas"];

// ---------------------------------------------------------------------------
// Enums & union types
// ---------------------------------------------------------------------------

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

/** How a run or workflow run was triggered. */
export type TriggerType = "manual" | "cron" | "spawn" | "workflow" | "retry";

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

/** Matches Go domain.EventType constants. */
export type EventType = "log" | "state_change" | "error" | "progress";

/** Webhook event type constants. */
export type WebhookEventType =
  | "run.completed"
  | "run.failed"
  | "run.timed_out"
  | "run.canceled"
  | "workflow.completed"
  | "workflow.failed";

/** Matches Go domain.FailurePolicy. */
export type FailurePolicy = "fail_workflow" | "skip_dependents" | "continue";

/** Matches Go domain.VersionPolicy. */
export type VersionPolicy = "pin" | "latest" | "minor";

/** Matches Go domain.RetryBackoffPolicy. */
export type RetryBackoffPolicy = "exponential" | "fixed";

/** Matches Go domain.CircuitState. */
export type CircuitState = "closed" | "open" | "half_open";

// ---------------------------------------------------------------------------
// Entity types — derived from Huma-generated OpenAPI component schemas
// ---------------------------------------------------------------------------

/** Timing breakdown for a job run execution. */
export type ExecutionTrace = Schema["ExecutionTrace"];

/** Rate limit key config embedded in Job. */
export type RateLimitKey = Schema["RateLimitKey"];

/** Job definition. */
export type Job = Schema["Job"];

/** Job run. */
export type JobRun = Schema["JobRun"];

/** Job group. */
export type JobGroup = Schema["JobGroup"];

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

/** Project role (RBAC). */
export type ProjectRole = Schema["ProjectRole"];

/** Region metadata from GET /v1/regions. */
export type Region = Schema["RegionResponse"];

/** API key (create response includes the key field). */
export type APIKey = Schema["CreateAPIKeyResponse"];

/** Response from POST /v1/api-keys/{keyID}/rotate. */
export type RotateAPIKeyResponse = Schema["RotateAPIKeyRequest"];

// ---------------------------------------------------------------------------
// Types not exposed as named Huma schemas (manual definitions)
// ---------------------------------------------------------------------------

/** JSON-safe value type for fields that can hold arbitrary JSON. */
export type JsonValue =
  | Record<string, never>
  | string
  | number
  | boolean
  | null;

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

/** Workflow step run. Not a named Huma schema. */
export type WorkflowStepRun = {
  id: string;
  workflow_run_id: string;
  workflow_step_id: string;
  step_ref: string;
  job_run_id: string;
  attempt: number;
  status: StepRunStatus;
  deps_completed: number;
  deps_required: number;
  output: JsonValue;
  error: string;
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
};

/** Audit event. Not a named Huma schema. */
export type AuditEvent = {
  id: string;
  project_id: string;
  actor_id: string;
  actor_type: string;
  action: string;
  resource_type: string;
  resource_id: string;
  details: JsonValue;
  created_at: string;
};

/** Endpoint circuit breaker state. */
export type EndpointCircuitState = {
  endpoint_url: string;
  state: CircuitState;
  consecutive_failures: number;
  opened_at: string | null;
  half_open_until: string | null;
  updated_at: string;
  created_at: string;
};

/** Environment. Matches Go domain.Environment. */
export type Environment = {
  id: string;
  project_id: string;
  name: string;
  slug: string;
  parent_id: string;
  variables: Record<string, string>;
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

/** Plan tier for region gating. Matches Go domain.PlanTier. */
export type PlanTier = "free" | "starter" | "pro" | "enterprise";

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
