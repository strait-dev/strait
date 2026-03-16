import { queryOptions, useMutation } from "@tanstack/react-query";
import type {
  PaginatedResponse,
  Workflow,
  WorkflowRun,
  WorkflowStep,
} from "@/hooks/api/types";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";

const now = new Date().toISOString();
const hourAgo = new Date(Date.now() - 3_600_000).toISOString();
const dayAgo = new Date(Date.now() - 86_400_000).toISOString();
const weekAgo = new Date(Date.now() - 7 * 86_400_000).toISOString();

const MOCK_WORKFLOWS: Workflow[] = [
  {
    id: "wf_user_onboarding",
    project_id: "proj_1",
    name: "User Onboarding",
    slug: "user-onboarding",
    description:
      "Provisions new user accounts, sends welcome email, and initializes default settings.",
    tags: { team: "growth", priority: "high" },
    enabled: true,
    version: 3,
    timeout_secs: 600,
    max_concurrent_runs: 10,
    max_parallel_steps: 4,
    cron: "",
    cron_timezone: "UTC",
    skip_if_running: false,
    version_id: "wfv_onb_3",
    version_policy: "latest",
    backwards_compatible: true,
    created_by: "user_1",
    updated_by: "user_1",
    created_at: weekAgo,
    updated_at: dayAgo,
  },
  {
    id: "wf_order_fulfillment",
    project_id: "proj_1",
    name: "Order Fulfillment",
    slug: "order-fulfillment",
    description:
      "Validates inventory, charges payment, and dispatches shipping for incoming orders.",
    tags: { team: "commerce", priority: "critical" },
    enabled: true,
    version: 7,
    timeout_secs: 1800,
    max_concurrent_runs: 50,
    max_parallel_steps: 6,
    cron: "",
    cron_timezone: "UTC",
    skip_if_running: false,
    version_id: "wfv_ord_7",
    version_policy: "latest",
    backwards_compatible: true,
    created_by: "user_1",
    updated_by: "user_2",
    created_at: weekAgo,
    updated_at: hourAgo,
  },
  {
    id: "wf_deploy_production",
    project_id: "proj_1",
    name: "Deploy Production",
    slug: "deploy-production",
    description:
      "Runs test suite, builds artifacts, deploys to production, and verifies health checks.",
    tags: { team: "platform", priority: "critical" },
    enabled: true,
    version: 12,
    timeout_secs: 3600,
    max_concurrent_runs: 1,
    max_parallel_steps: 3,
    cron: "",
    cron_timezone: "UTC",
    skip_if_running: true,
    version_id: "wfv_dep_12",
    version_policy: "pin",
    backwards_compatible: false,
    created_by: "user_2",
    updated_by: "user_2",
    created_at: weekAgo,
    updated_at: dayAgo,
  },
  {
    id: "wf_data_pipeline",
    project_id: "proj_1",
    name: "Data Pipeline",
    slug: "data-pipeline",
    description:
      "Extracts data from source systems, transforms and validates it, then loads into the warehouse.",
    tags: { team: "data", priority: "high" },
    enabled: true,
    version: 5,
    timeout_secs: 7200,
    max_concurrent_runs: 2,
    max_parallel_steps: 8,
    cron: "0 2 * * *",
    cron_timezone: "America/New_York",
    skip_if_running: true,
    version_id: "wfv_pip_5",
    version_policy: "minor",
    backwards_compatible: true,
    created_by: "user_3",
    updated_by: "user_3",
    created_at: weekAgo,
    updated_at: dayAgo,
  },
  {
    id: "wf_subscription_renewal",
    project_id: "proj_1",
    name: "Subscription Renewal",
    slug: "subscription-renewal",
    description:
      "Checks upcoming renewals, processes payments, and sends confirmation or retry notices.",
    tags: { team: "billing", priority: "high" },
    enabled: false,
    version: 2,
    timeout_secs: 900,
    max_concurrent_runs: 5,
    max_parallel_steps: 3,
    cron: "0 6 * * *",
    cron_timezone: "UTC",
    skip_if_running: false,
    version_id: "wfv_sub_2",
    version_policy: "latest",
    backwards_compatible: true,
    created_by: "user_1",
    updated_by: "user_1",
    created_at: weekAgo,
    updated_at: weekAgo,
  },
];

// Steps for user-onboarding workflow
const MOCK_STEPS_ONBOARDING: WorkflowStep[] = [
  {
    id: "ws_onb_1",
    workflow_id: "wf_user_onboarding",
    job_id: "job_validate_email",
    step_ref: "validate-email",
    depends_on: [],
    condition: null,
    on_failure: "fail_workflow",
    payload: null,
    step_type: "job",
    approval_timeout_secs: 0,
    approval_approvers: [],
    retry_max_attempts: 3,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 2,
    retry_max_delay_secs: 30,
    timeout_secs_override: 60,
    output_transform: "",
    sub_workflow_id: "",
    max_nesting_depth: 0,
    event_key: "",
    event_timeout_secs: 0,
    event_notify_url: "",
    sleep_duration_secs: 0,
    event_emit_key: "",
    concurrency_key: "",
    resource_class: "default",
    created_at: weekAgo,
  },
  {
    id: "ws_onb_2",
    workflow_id: "wf_user_onboarding",
    job_id: "job_create_account",
    step_ref: "create-account",
    depends_on: ["validate-email"],
    condition: null,
    on_failure: "fail_workflow",
    payload: null,
    step_type: "job",
    approval_timeout_secs: 0,
    approval_approvers: [],
    retry_max_attempts: 2,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 5,
    retry_max_delay_secs: 5,
    timeout_secs_override: 120,
    output_transform: "",
    sub_workflow_id: "",
    max_nesting_depth: 0,
    event_key: "",
    event_timeout_secs: 0,
    event_notify_url: "",
    sleep_duration_secs: 0,
    event_emit_key: "",
    concurrency_key: "",
    resource_class: "default",
    created_at: weekAgo,
  },
  {
    id: "ws_onb_3",
    workflow_id: "wf_user_onboarding",
    job_id: "job_send_welcome",
    step_ref: "send-welcome-email",
    depends_on: ["create-account"],
    condition: null,
    on_failure: "continue",
    payload: null,
    step_type: "job",
    approval_timeout_secs: 0,
    approval_approvers: [],
    retry_max_attempts: 3,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 5,
    retry_max_delay_secs: 60,
    timeout_secs_override: 30,
    output_transform: "",
    sub_workflow_id: "",
    max_nesting_depth: 0,
    event_key: "",
    event_timeout_secs: 0,
    event_notify_url: "",
    sleep_duration_secs: 0,
    event_emit_key: "",
    concurrency_key: "",
    resource_class: "default",
    created_at: weekAgo,
  },
  {
    id: "ws_onb_4",
    workflow_id: "wf_user_onboarding",
    job_id: "job_init_defaults",
    step_ref: "initialize-defaults",
    depends_on: ["create-account"],
    condition: null,
    on_failure: "skip_dependents",
    payload: null,
    step_type: "job",
    approval_timeout_secs: 0,
    approval_approvers: [],
    retry_max_attempts: 1,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 0,
    retry_max_delay_secs: 0,
    timeout_secs_override: 90,
    output_transform: "",
    sub_workflow_id: "",
    max_nesting_depth: 0,
    event_key: "",
    event_timeout_secs: 0,
    event_notify_url: "",
    sleep_duration_secs: 0,
    event_emit_key: "",
    concurrency_key: "",
    resource_class: "default",
    created_at: weekAgo,
  },
];

// Steps for deploy-production workflow
const MOCK_STEPS_DEPLOY: WorkflowStep[] = [
  {
    id: "ws_dep_1",
    workflow_id: "wf_deploy_production",
    job_id: "job_run_tests",
    step_ref: "run-tests",
    depends_on: [],
    condition: null,
    on_failure: "fail_workflow",
    payload: null,
    step_type: "job",
    approval_timeout_secs: 0,
    approval_approvers: [],
    retry_max_attempts: 1,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 0,
    retry_max_delay_secs: 0,
    timeout_secs_override: 600,
    output_transform: "",
    sub_workflow_id: "",
    max_nesting_depth: 0,
    event_key: "",
    event_timeout_secs: 0,
    event_notify_url: "",
    sleep_duration_secs: 0,
    event_emit_key: "",
    concurrency_key: "",
    resource_class: "large",
    created_at: weekAgo,
  },
  {
    id: "ws_dep_2",
    workflow_id: "wf_deploy_production",
    job_id: "job_build_artifacts",
    step_ref: "build-artifacts",
    depends_on: ["run-tests"],
    condition: null,
    on_failure: "fail_workflow",
    payload: null,
    step_type: "job",
    approval_timeout_secs: 0,
    approval_approvers: [],
    retry_max_attempts: 2,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 10,
    retry_max_delay_secs: 60,
    timeout_secs_override: 900,
    output_transform: "",
    sub_workflow_id: "",
    max_nesting_depth: 0,
    event_key: "",
    event_timeout_secs: 0,
    event_notify_url: "",
    sleep_duration_secs: 0,
    event_emit_key: "",
    concurrency_key: "",
    resource_class: "large",
    created_at: weekAgo,
  },
  {
    id: "ws_dep_3",
    workflow_id: "wf_deploy_production",
    job_id: "",
    step_ref: "approve-deploy",
    depends_on: ["build-artifacts"],
    condition: null,
    on_failure: "fail_workflow",
    payload: null,
    step_type: "approval",
    approval_timeout_secs: 3600,
    approval_approvers: ["user_1", "user_2"],
    retry_max_attempts: 0,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 0,
    retry_max_delay_secs: 0,
    timeout_secs_override: 0,
    output_transform: "",
    sub_workflow_id: "",
    max_nesting_depth: 0,
    event_key: "",
    event_timeout_secs: 0,
    event_notify_url: "",
    sleep_duration_secs: 0,
    event_emit_key: "",
    concurrency_key: "",
    resource_class: "",
    created_at: weekAgo,
  },
  {
    id: "ws_dep_4",
    workflow_id: "wf_deploy_production",
    job_id: "job_deploy",
    step_ref: "deploy",
    depends_on: ["approve-deploy"],
    condition: null,
    on_failure: "fail_workflow",
    payload: null,
    step_type: "job",
    approval_timeout_secs: 0,
    approval_approvers: [],
    retry_max_attempts: 1,
    retry_backoff: "fixed",
    retry_initial_delay_secs: 0,
    retry_max_delay_secs: 0,
    timeout_secs_override: 300,
    output_transform: "",
    sub_workflow_id: "",
    max_nesting_depth: 0,
    event_key: "",
    event_timeout_secs: 0,
    event_notify_url: "",
    sleep_duration_secs: 0,
    event_emit_key: "",
    concurrency_key: "",
    resource_class: "default",
    created_at: weekAgo,
  },
  {
    id: "ws_dep_5",
    workflow_id: "wf_deploy_production",
    job_id: "job_health_check",
    step_ref: "health-check",
    depends_on: ["deploy"],
    condition: null,
    on_failure: "fail_workflow",
    payload: null,
    step_type: "job",
    approval_timeout_secs: 0,
    approval_approvers: [],
    retry_max_attempts: 5,
    retry_backoff: "exponential",
    retry_initial_delay_secs: 5,
    retry_max_delay_secs: 120,
    timeout_secs_override: 180,
    output_transform: "",
    sub_workflow_id: "",
    max_nesting_depth: 0,
    event_key: "",
    event_timeout_secs: 0,
    event_notify_url: "",
    sleep_duration_secs: 0,
    event_emit_key: "",
    concurrency_key: "",
    resource_class: "default",
    created_at: weekAgo,
  },
];

const MOCK_STEPS: Record<string, WorkflowStep[]> = {
  wf_user_onboarding: MOCK_STEPS_ONBOARDING,
  wf_deploy_production: MOCK_STEPS_DEPLOY,
};

const MOCK_WORKFLOW_RUNS: Record<string, WorkflowRun[]> = {
  wf_user_onboarding: [
    {
      id: "wfr_onb_1",
      workflow_id: "wf_user_onboarding",
      project_id: "proj_1",
      tags: { user: "u_42" },
      status: "completed",
      triggered_by: "manual",
      workflow_version: 3,
      max_parallel_steps: 4,
      payload: { email: "new@example.com" },
      error: "",
      started_at: hourAgo,
      finished_at: now,
      expires_at: null,
      retry_of_run_id: "",
      parent_workflow_run_id: "",
      parent_step_run_id: "",
      workflow_version_id: "wfv_onb_3",
      created_by: "user_1",
      created_at: hourAgo,
    },
    {
      id: "wfr_onb_2",
      workflow_id: "wf_user_onboarding",
      project_id: "proj_1",
      tags: { user: "u_43" },
      status: "running",
      triggered_by: "manual",
      workflow_version: 3,
      max_parallel_steps: 4,
      payload: { email: "another@example.com" },
      error: "",
      started_at: now,
      finished_at: null,
      expires_at: null,
      retry_of_run_id: "",
      parent_workflow_run_id: "",
      parent_step_run_id: "",
      workflow_version_id: "wfv_onb_3",
      created_by: "user_1",
      created_at: now,
    },
  ],
  wf_deploy_production: [
    {
      id: "wfr_dep_1",
      workflow_id: "wf_deploy_production",
      project_id: "proj_1",
      tags: { branch: "main", commit: "abc123" },
      status: "completed",
      triggered_by: "manual",
      workflow_version: 12,
      max_parallel_steps: 3,
      payload: { ref: "main" },
      error: "",
      started_at: dayAgo,
      finished_at: dayAgo,
      expires_at: null,
      retry_of_run_id: "",
      parent_workflow_run_id: "",
      parent_step_run_id: "",
      workflow_version_id: "wfv_dep_12",
      created_by: "user_2",
      created_at: dayAgo,
    },
    {
      id: "wfr_dep_2",
      workflow_id: "wf_deploy_production",
      project_id: "proj_1",
      tags: { branch: "main", commit: "def456" },
      status: "failed",
      triggered_by: "manual",
      workflow_version: 12,
      max_parallel_steps: 3,
      payload: { ref: "main" },
      error: "Health check failed: /api/health returned 503",
      started_at: hourAgo,
      finished_at: now,
      expires_at: null,
      retry_of_run_id: "",
      parent_workflow_run_id: "",
      parent_step_run_id: "",
      workflow_version_id: "wfv_dep_12",
      created_by: "user_2",
      created_at: hourAgo,
    },
  ],
  wf_data_pipeline: [
    {
      id: "wfr_pip_1",
      workflow_id: "wf_data_pipeline",
      project_id: "proj_1",
      tags: { source: "postgres" },
      status: "completed",
      triggered_by: "cron",
      workflow_version: 5,
      max_parallel_steps: 8,
      payload: null,
      error: "",
      started_at: dayAgo,
      finished_at: dayAgo,
      expires_at: null,
      retry_of_run_id: "",
      parent_workflow_run_id: "",
      parent_step_run_id: "",
      workflow_version_id: "wfv_pip_5",
      created_by: "system",
      created_at: dayAgo,
    },
  ],
};

function filterWorkflows(search?: string): PaginatedResponse<Workflow> {
  let filtered = MOCK_WORKFLOWS;
  if (search) {
    const q = search.toLowerCase();
    filtered = MOCK_WORKFLOWS.filter(
      (w) =>
        w.name.toLowerCase().includes(q) ||
        w.slug.toLowerCase().includes(q) ||
        w.description.toLowerCase().includes(q)
    );
  }
  return { data: filtered, page_count: 1, total_count: filtered.length };
}

function findWorkflow(id: string): Workflow | null {
  return MOCK_WORKFLOWS.find((w) => w.id === id) ?? null;
}

function getSteps(workflowId: string): WorkflowStep[] {
  return MOCK_STEPS[workflowId] ?? [];
}

function getRuns(workflowId: string): WorkflowRun[] {
  return MOCK_WORKFLOW_RUNS[workflowId] ?? [];
}

/** List workflows, optionally filtered by search string. */
export const workflowsQueryOptions = (search?: string) =>
  queryOptions({
    queryKey: ["workflows", { search }],
    queryFn: () => Promise.resolve(filterWorkflows(search)),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Fetch a single workflow by id. */
export const workflowQueryOptions = (id: string) =>
  queryOptions({
    queryKey: ["workflows", id],
    queryFn: () => Promise.resolve(findWorkflow(id)),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Fetch the DAG steps for a workflow. */
export const workflowStepsQueryOptions = (workflowId: string) =>
  queryOptions({
    queryKey: ["workflows", workflowId, "steps"],
    queryFn: () => Promise.resolve(getSteps(workflowId)),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Fetch runs for a workflow. */
export const workflowRunsQueryOptions = (workflowId: string) =>
  queryOptions({
    queryKey: ["workflows", workflowId, "runs"],
    queryFn: () => Promise.resolve(getRuns(workflowId)),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

/** Trigger a new run of a workflow. */
export const useTriggerWorkflow = () =>
  useMutation({
    mutationKey: ["workflows", "trigger"],
    mutationFn: async (params: {
      workflowId: string;
      payload?: unknown;
    }): Promise<WorkflowRun> => {
      // Mock: return a new running workflow run
      await Promise.resolve();
      const workflow = findWorkflow(params.workflowId);
      const id = `wfr_${Date.now().toString(36)}`;
      return {
        id,
        workflow_id: params.workflowId,
        project_id: workflow?.project_id ?? "proj_1",
        tags: {},
        status: "running",
        triggered_by: "manual",
        workflow_version: workflow?.version ?? 1,
        max_parallel_steps: workflow?.max_parallel_steps ?? 1,
        payload: params.payload ?? null,
        error: "",
        started_at: new Date().toISOString(),
        finished_at: null,
        expires_at: null,
        retry_of_run_id: "",
        parent_workflow_run_id: "",
        parent_step_run_id: "",
        workflow_version_id: workflow?.version_id ?? "",
        created_by: "user_1",
        created_at: new Date().toISOString(),
      };
    },
  });

/** Pause a running workflow. */
export const usePauseWorkflow = () =>
  useMutation({
    mutationKey: ["workflows", "pause"],
    mutationFn: async (params: { workflowId: string }): Promise<Workflow> => {
      await Promise.resolve();
      const workflow = findWorkflow(params.workflowId);
      if (!workflow) {
        throw new Error(`Workflow ${params.workflowId} not found`);
      }
      return {
        ...workflow,
        enabled: false,
        updated_at: new Date().toISOString(),
      };
    },
  });

/** Resume a paused workflow. */
export const useResumeWorkflow = () =>
  useMutation({
    mutationKey: ["workflows", "resume"],
    mutationFn: async (params: { workflowId: string }): Promise<Workflow> => {
      await Promise.resolve();
      const workflow = findWorkflow(params.workflowId);
      if (!workflow) {
        throw new Error(`Workflow ${params.workflowId} not found`);
      }
      return {
        ...workflow,
        enabled: true,
        updated_at: new Date().toISOString(),
      };
    },
  });
