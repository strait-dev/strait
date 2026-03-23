// Canonical frontend types for the orchestration data model.
// All entity types are derived from the generated OpenAPI schema (lib/api/schema.d.ts).
// Run `bun run generate:api` to regenerate the schema from openapi.yaml.

import type { components } from "@/lib/api/schema";

// ---------------------------------------------------------------------------
// Helper: make all fields required (the spec marks most as optional because
// Go omitempty makes them nullable in JSON, but the backend always returns them)
// ---------------------------------------------------------------------------

type Strict<T> = { [K in keyof T]-?: NonNullable<T[K]> };

// ---------------------------------------------------------------------------
// Enums & union types (re-exported from generated schema)
// ---------------------------------------------------------------------------

export type RunStatus = components["schemas"]["RunStatus"];

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
// Entity types — derived from generated OpenAPI component schemas
// ---------------------------------------------------------------------------

/** Timing breakdown for a job run execution. */
export type ExecutionTrace = Strict<components["schemas"]["ExecutionTrace"]>;

/** Rate limit key config embedded in Job. */
export type RateLimitKey = Strict<components["schemas"]["RateLimitKey"]>;

/** Job definition. */
export type Job = {
  [K in keyof components["schemas"]["Job"]]-?: K extends
    | "payload_schema"
    | "tags"
    | "rate_limit_keys"
    | "default_run_metadata"
    | "retry_delays_secs"
    ? NonNullable<components["schemas"]["Job"][K]>
    : K extends "version_policy"
      ? VersionPolicy
      : NonNullable<components["schemas"]["Job"][K]>;
};

/** Job run. */
export type JobRun = {
  [K in keyof components["schemas"]["JobRun"]]-?: K extends
    | "scheduled_at"
    | "started_at"
    | "finished_at"
    | "heartbeat_at"
    | "next_retry_at"
    | "expires_at"
    | "execution_trace"
    ? components["schemas"]["JobRun"][K] | null
    : K extends "status"
      ? RunStatus
      : K extends "tags" | "metadata"
        ? Record<string, string>
        : NonNullable<components["schemas"]["JobRun"][K]>;
};

/** Job group. */
export type JobGroup = Strict<components["schemas"]["JobGroup"]>;

/** Workflow DAG definition. */
export type Workflow = {
  [K in keyof components["schemas"]["Workflow"]]-?: K extends "version_policy"
    ? VersionPolicy
    : K extends "tags"
      ? Record<string, string>
      : NonNullable<components["schemas"]["Workflow"][K]>;
};

/** Workflow step (node in a workflow DAG). */
export type WorkflowStep = Strict<components["schemas"]["WorkflowStep"]>;

/** Workflow run. */
export type WorkflowRun = {
  [K in keyof components["schemas"]["WorkflowRun"]]-?: K extends
    | "started_at"
    | "finished_at"
    | "expires_at"
    ? components["schemas"]["WorkflowRun"][K] | null
    : K extends "status"
      ? WorkflowRunStatus
      : K extends "tags"
        ? Record<string, string>
        : NonNullable<components["schemas"]["WorkflowRun"][K]>;
};

/** Workflow step run. */
export type WorkflowStepRun = {
  [K in keyof components["schemas"]["WorkflowStepRun"]]-?: K extends
    | "started_at"
    | "finished_at"
    ? components["schemas"]["WorkflowStepRun"][K] | null
    : K extends "status"
      ? StepRunStatus
      : NonNullable<components["schemas"]["WorkflowStepRun"][K]>;
};

/** Webhook subscription. */
export type WebhookSubscription = Strict<
  components["schemas"]["WebhookSubscription"]
>;

/** Webhook delivery. */
export type WebhookDelivery = {
  [K in keyof components["schemas"]["WebhookDelivery"]]-?: K extends
    | "last_status_code"
    | "next_retry_at"
    | "delivered_at"
    ? components["schemas"]["WebhookDelivery"][K] | null
    : NonNullable<components["schemas"]["WebhookDelivery"][K]>;
};

/** Run event. */
export type RunEvent = Strict<components["schemas"]["RunEvent"]>;

/** API key. */
export type APIKey = {
  [K in keyof components["schemas"]["APIKey"]]-?: K extends
    | "expires_at"
    | "last_used_at"
    | "revoked_at"
    | "grace_expires_at"
    | "next_rotation_at"
    | "rotation_interval_days"
    ? components["schemas"]["APIKey"][K] | null
    : NonNullable<components["schemas"]["APIKey"][K]>;
};

/** Response from POST /v1/api-keys/{keyID}/rotate. */
export type RotateAPIKeyResponse = Strict<
  components["schemas"]["RotateAPIKeyResponse"]
>;

/** Event trigger (durable wait). */
export type EventTrigger = {
  [K in keyof components["schemas"]["EventTrigger"]]-?: K extends
    | "workflow_run_id"
    | "workflow_step_run_id"
    | "job_run_id"
    | "received_at"
    | "error"
    | "request_payload"
    | "response_payload"
    ? components["schemas"]["EventTrigger"][K] | null
    : NonNullable<components["schemas"]["EventTrigger"][K]>;
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

/** Audit event. */
export type AuditEvent = Strict<components["schemas"]["AuditEvent"]>;

/** Project role (RBAC). */
export type ProjectRole = Strict<components["schemas"]["ProjectRole"]>;

/** Region metadata from GET /v1/regions. */
export type Region = Strict<components["schemas"]["Region"]>;

// ---------------------------------------------------------------------------
// Types not in the OpenAPI spec (frontend-only or from other services)
// ---------------------------------------------------------------------------

/** JSON-safe value type for fields that can hold arbitrary JSON. */
export type JsonValue =
  | Record<string, never>
  | string
  | number
  | boolean
  | null;

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
export type ProjectSettings = {
  project_id: string;
  default_region: string;
  plan_tier: PlanTier;
};

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
