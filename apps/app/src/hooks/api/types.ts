// Canonical frontend types for the orchestration data model.
// Field names use snake_case to match Go JSON tags exactly — no mapping layer needed.
// All timestamps are ISO 8601 strings; nullable Go *time.Time maps to string | null.

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
  | "dead_letter"
  | "replay_staged";

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

/** Timing breakdown for a job run execution. Matches Go domain.ExecutionTrace. */
export type ExecutionTrace = {
  queue_wait_ms: number;
  dequeue_ms: number;
  connect_ms: number;
  ttfb_ms: number;
  transfer_ms: number;
  total_ms: number;
  dispatch_ms: number;
};

/** Rate limit key config embedded in Job. */
export type RateLimitKey = {
  name: string;
  max: number;
  window_secs: number;
};

/** Job definition. Matches Go domain.Job. */
export type Job = {
  id: string;
  project_id: string;
  group_id: string;
  name: string;
  slug: string;
  description: string;
  cron: string;
  payload_schema: unknown;
  tags: Record<string, string>;
  endpoint_url: string;
  fallback_endpoint_url: string;
  max_attempts: number;
  timeout_secs: number;
  max_concurrency: number;
  max_concurrency_per_key: number;
  execution_window_cron: string;
  timezone: string;
  rate_limit_max: number;
  rate_limit_window_secs: number;
  rate_limit_keys: RateLimitKey[];
  dedup_window_secs: number;
  enabled: boolean;
  webhook_url: string;
  webhook_secret: string;
  run_ttl_secs: number;
  retry_strategy: string;
  retry_delays_secs: number[];
  environment_id: string;
  default_run_metadata: Record<string, string>;
  version: number;
  version_id: string;
  version_policy: VersionPolicy;
  backwards_compatible: boolean;
  created_by: string;
  updated_by: string;
  created_at: string; // ISO 8601
  updated_at: string;
};

/** Job run. Matches Go domain.JobRun. */
export type JobRun = {
  id: string;
  job_id: string;
  project_id: string;
  tags: Record<string, string>;
  status: RunStatus;
  attempt: number;
  payload: unknown;
  result: unknown;
  metadata: Record<string, string>;
  error: string;
  triggered_by: TriggerType;
  scheduled_at: string | null;
  started_at: string | null;
  finished_at: string | null;
  heartbeat_at: string | null;
  next_retry_at: string | null;
  expires_at: string | null;
  parent_run_id: string;
  priority: number;
  idempotency_key: string;
  job_version: number;
  job_version_id: string;
  workflow_step_run_id: string;
  max_attempts_override: number;
  timeout_secs_override: number;
  retry_backoff: string;
  retry_initial_delay_secs: number;
  retry_max_delay_secs: number;
  execution_trace: ExecutionTrace | null;
  debug_mode: boolean;
  continuation_of: string;
  lineage_depth: number;
  created_by: string;
  batch_id: string;
  concurrency_key: string;
  created_at: string;
};

/** Job group. Matches Go domain.JobGroup. */
export type JobGroup = {
  id: string;
  project_id: string;
  name: string;
  slug: string;
  description: string;
  created_at: string;
  updated_at: string;
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

/** Workflow DAG definition. Matches Go domain.Workflow. */
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

/** Workflow step (node in a workflow DAG). Matches Go domain.WorkflowStep. */
export type WorkflowStep = {
  id: string;
  workflow_id: string;
  job_id: string;
  step_ref: string;
  depends_on: string[];
  condition: unknown;
  on_failure: FailurePolicy;
  payload: unknown;
  step_type: WorkflowStepType;
  approval_timeout_secs: number;
  approval_approvers: string[];
  retry_max_attempts: number;
  retry_backoff: RetryBackoffPolicy;
  retry_initial_delay_secs: number;
  retry_max_delay_secs: number;
  timeout_secs_override: number;
  output_transform: string;
  sub_workflow_id: string;
  max_nesting_depth: number;
  event_key: string;
  event_timeout_secs: number;
  event_notify_url: string;
  sleep_duration_secs: number;
  event_emit_key: string;
  concurrency_key: string;
  resource_class: string;
  created_at: string;
};

/** Workflow run. Matches Go domain.WorkflowRun. */
export type WorkflowRun = {
  id: string;
  workflow_id: string;
  project_id: string;
  tags: Record<string, string>;
  status: WorkflowRunStatus;
  triggered_by: TriggerType;
  workflow_version: number;
  max_parallel_steps: number;
  payload: unknown;
  error: string;
  started_at: string | null;
  finished_at: string | null;
  expires_at: string | null;
  retry_of_run_id: string;
  parent_workflow_run_id: string;
  parent_step_run_id: string;
  workflow_version_id: string;
  created_by: string;
  created_at: string;
};

/** Workflow step run. Matches Go domain.WorkflowStepRun. */
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
  output: unknown;
  error: string;
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
};

/** Webhook subscription. Matches Go domain.WebhookSubscription. */
export type WebhookSubscription = {
  id: string;
  project_id: string;
  webhook_url: string;
  event_types: string[];
  secret: string;
  active: boolean;
  created_at: string;
};

/** Webhook delivery. Matches Go domain.WebhookDelivery. */
export type WebhookDelivery = {
  id: string;
  run_id: string;
  job_id: string;
  event_trigger_id: string;
  webhook_url: string;
  webhook_retry_policy: string;
  status: string;
  attempts: number;
  max_attempts: number;
  last_status_code: number | null;
  last_error: string;
  next_retry_at: string | null;
  delivered_at: string | null;
  created_at: string;
  updated_at: string;
};

/** Run event. Matches Go domain.RunEvent. */
export type RunEvent = {
  id: string;
  run_id: string;
  type: EventType;
  level: string;
  message: string;
  data: unknown;
  created_at: string;
};

/** API key. Matches Go domain.APIKey (key_hash excluded via json:"-"). */
export type APIKey = {
  id: string;
  project_id: string;
  name: string;
  key_prefix: string;
  scopes: string[];
  expires_at: string | null;
  last_used_at: string | null;
  created_at: string;
  revoked_at: string | null;
  replaced_by_key_id: string;
  grace_expires_at: string | null;
};

/** Endpoint circuit breaker state. Matches Go domain.EndpointCircuitState. */
export type EndpointCircuitState = {
  endpoint_url: string;
  state: CircuitState;
  consecutive_failures: number;
  opened_at: string | null;
  half_open_until: string | null;
  updated_at: string;
  created_at: string;
};

/** Audit event. Matches Go domain.AuditEvent. */
export type AuditEvent = {
  id: string;
  project_id: string;
  actor_id: string;
  actor_type: string;
  action: string;
  resource_type: string;
  resource_id: string;
  details: unknown;
  created_at: string;
};

/** Project role (RBAC). Matches Go domain.ProjectRole. */
export type ProjectRole = {
  id: string;
  project_id: string;
  name: string;
  description: string;
  permissions: string[];
  parent_role_id: string;
  is_system: boolean;
  created_at: string;
  updated_at: string;
};

/** Event trigger (durable wait). Matches Go domain.EventTrigger. */
export type EventTrigger = {
  id: string;
  event_key: string;
  project_id: string;
  source_type: string;
  workflow_run_id: string;
  workflow_step_run_id: string;
  job_run_id: string;
  status: string;
  request_payload: unknown;
  response_payload: unknown;
  timeout_secs: number;
  requested_at: string;
  received_at: string | null;
  expires_at: string;
  error: string;
  notify_url: string;
  notify_status: string;
  trigger_type: string;
  sent_by: string;
};

/** Union of RunStatus and WorkflowRunStatus, used by StatusBadge. */
export type DisplayStatus = RunStatus | WorkflowRunStatus;

/** Paginated response wrapper matching the API envelope. */
export type PaginatedResponse<T> = {
  data: T[];
  page_count: number;
  total_count: number;
};

/** Common search/filter params for list endpoints. */
export type ListParams = {
  query?: string;
  page?: number;
  per_page?: number;
  sort?: "asc" | "desc";
};
