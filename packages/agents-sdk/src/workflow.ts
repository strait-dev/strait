import { StraitSDKError } from "./errors";
import type { JsonValue } from "./types";

export type WorkflowFailurePolicy =
  | "continue"
  | "fail_workflow"
  | "skip_dependents";

export type WorkflowStepType =
  | "approval"
  | "job"
  | "sleep"
  | "sub_workflow"
  | "wait_for_event";

export type WorkflowResourceClass = "large" | "medium" | "small";
export type WorkflowRetryBackoff = "exponential" | "fixed";

export interface WorkflowRetryOptions {
  backoff?: WorkflowRetryBackoff;
  initialDelaySecs?: number;
  maxAttempts?: number;
  maxDelaySecs?: number;
}

interface SharedStepOptions {
  concurrencyKey?: string;
  condition?: JsonValue;
  expectedDurationSecs?: number;
  onFailure?: WorkflowFailurePolicy;
  outputTransform?: string;
  payload?: JsonValue;
  resourceClass?: WorkflowResourceClass;
  retry?: WorkflowRetryOptions;
  timeoutSecsOverride?: number;
}

interface DependentStepOptions extends SharedStepOptions {
  dependsOn?: string[];
}

export interface AgentWorkflowStepDefinition {
  agent_id?: string;
  approval_approvers?: string[];
  approval_timeout_secs?: number;
  compensation_job_id?: string;
  compensation_timeout_secs?: number;
  concurrency_key?: string;
  condition?: JsonValue;
  depends_on?: string[];
  event_emit_key?: string;
  event_key?: string;
  event_notify_url?: string;
  event_timeout_secs?: number;
  expected_duration_secs?: number;
  job_id?: string;
  max_nesting_depth?: number;
  on_failure?: WorkflowFailurePolicy;
  output_transform?: string;
  payload?: JsonValue;
  resource_class?: WorkflowResourceClass;
  retry_backoff?: WorkflowRetryBackoff;
  retry_initial_delay_secs?: number;
  retry_max_attempts?: number;
  retry_max_delay_secs?: number;
  sleep_duration_secs?: number;
  stage_notifications?: JsonValue;
  step_ref: string;
  step_type?: WorkflowStepType;
  sub_workflow_id?: string;
  timeout_secs_override?: number;
}

export interface DynamicWorkflowStepOptions {
  knownStepRefs?: string[];
  maxDynamicSteps?: number;
}

export interface DynamicWorkflowStepEnvelope {
  dynamic_steps: AgentWorkflowStepDefinition[];
}

export interface AgentWorkflowDefinition {
  description?: string;
  max_parallel_steps?: number;
  name: string;
  project_id: string;
  slug: string;
  steps: AgentWorkflowStepDefinition[];
}

export interface AgentStepOptions extends DependentStepOptions {
  compensationJobId?: string;
  compensationTimeoutSecs?: number;
  stageNotifications?: JsonValue;
}

export interface ApprovalStepOptions extends DependentStepOptions {
  approvers?: string[];
  timeoutSecs?: number;
}

export interface SleepStepOptions extends DependentStepOptions {
  durationSecs: number;
}

export interface SubWorkflowStepOptions extends DependentStepOptions {
  maxNestingDepth?: number;
  subWorkflowId: string;
}

export interface WaitForEventStepOptions extends DependentStepOptions {
  eventEmitKey?: string;
  eventKey: string;
  eventNotifyUrl?: string;
  eventTimeoutSecs?: number;
}

export interface PipelinePatternStep {
  agentId?: string;
  approvers?: string[];
  payload?: JsonValue;
  stepRef: string;
  timeoutSecs?: number;
  type?: "agent" | "approval";
}

export interface PipelinePatternDefinition {
  description?: string;
  name: string;
  projectId: string;
  slug: string;
  steps: PipelinePatternStep[];
}

export interface DebatePatternDefinition {
  analysts: Array<{
    agentId: string;
    payload?: JsonValue;
    stepRef: string;
  }>;
  description?: string;
  judge: {
    agentId: string;
    payload?: JsonValue;
    stepRef: string;
  };
  name: string;
  projectId: string;
  slug: string;
}

export interface OrchestratorPatternDefinition {
  description?: string;
  name: string;
  planner: {
    agentId: string;
    payload?: JsonValue;
    stepRef: string;
  };
  projectId: string;
  slug: string;
  synthesizer: {
    agentId: string;
    payload?: JsonValue;
    stepRef: string;
  };
  workers: Array<{
    agentId: string;
    payload?: JsonValue;
    stepRef: string;
  }>;
}

export interface FanOutWorkerDefinition {
  agentId: string;
  payload?: JsonValue;
  stepRef?: string;
}

export interface FanOutStepsDefinition {
  dependsOn?: string[];
  stepRefPrefix?: string;
  synthesizer?: {
    agentId: string;
    payload?: JsonValue;
    stepRef: string;
  };
  workers: FanOutWorkerDefinition[];
}

const defaultDynamicWorkflowStepLimit = 100;

function requireNonEmpty(value: string, field: string): string {
  const normalized = value.trim();
  if (normalized.length === 0) {
    throw new StraitSDKError(`${field} is required`);
  }
  return normalized;
}

function optionalNonEmpty(
  value: string | undefined,
  field: string
): string | undefined {
  if (value == null) {
    return undefined;
  }
  return requireNonEmpty(value, field);
}

function assertNonNegativeInt(
  value: number | undefined,
  field: string
): number | undefined {
  if (value == null) {
    return undefined;
  }
  if (!Number.isInteger(value) || value < 0) {
    throw new StraitSDKError(`${field} must be a non-negative integer`);
  }
  return value;
}

function normalizeDependsOn(
  dependsOn: string[] | undefined
): string[] | undefined {
  if (dependsOn == null || dependsOn.length === 0) {
    return undefined;
  }

  return dependsOn.map((stepRef, index) =>
    requireNonEmpty(stepRef, `dependsOn[${index}]`)
  );
}

function normalizeRetry(
  retry: WorkflowRetryOptions | undefined
): Pick<
  AgentWorkflowStepDefinition,
  | "retry_backoff"
  | "retry_initial_delay_secs"
  | "retry_max_attempts"
  | "retry_max_delay_secs"
> {
  return {
    retry_backoff: retry?.backoff,
    retry_initial_delay_secs: assertNonNegativeInt(
      retry?.initialDelaySecs,
      "retry.initialDelaySecs"
    ),
    retry_max_attempts: assertNonNegativeInt(
      retry?.maxAttempts,
      "retry.maxAttempts"
    ),
    retry_max_delay_secs: assertNonNegativeInt(
      retry?.maxDelaySecs,
      "retry.maxDelaySecs"
    ),
  };
}

function normalizeSharedOptions<T extends SharedStepOptions>(
  options: T
): Pick<
  AgentWorkflowStepDefinition,
  | "concurrency_key"
  | "condition"
  | "expected_duration_secs"
  | "on_failure"
  | "output_transform"
  | "payload"
  | "resource_class"
  | "timeout_secs_override"
> &
  Pick<
    AgentWorkflowStepDefinition,
    | "retry_backoff"
    | "retry_initial_delay_secs"
    | "retry_max_attempts"
    | "retry_max_delay_secs"
  > {
  if (
    options.resourceClass != null &&
    options.resourceClass !== "small" &&
    options.resourceClass !== "medium" &&
    options.resourceClass !== "large"
  ) {
    throw new StraitSDKError(
      "resourceClass must be one of: small, medium, large"
    );
  }

  return {
    concurrency_key: optionalNonEmpty(options.concurrencyKey, "concurrencyKey"),
    condition: options.condition,
    expected_duration_secs: assertNonNegativeInt(
      options.expectedDurationSecs,
      "expectedDurationSecs"
    ),
    on_failure: options.onFailure,
    output_transform: optionalNonEmpty(
      options.outputTransform,
      "outputTransform"
    ),
    payload: options.payload,
    resource_class: options.resourceClass,
    timeout_secs_override: assertNonNegativeInt(
      options.timeoutSecsOverride,
      "timeoutSecsOverride"
    ),
    ...normalizeRetry(options.retry),
  };
}

function normalizeAgentWorkflowStep(
  step: AgentWorkflowStepDefinition
): AgentWorkflowStepDefinition {
  const normalizedStepType = step.step_type ?? "job";
  const normalized: AgentWorkflowStepDefinition = {
    ...step,
    agent_id: optionalNonEmpty(step.agent_id, "agent_id"),
    approval_approvers: step.approval_approvers?.map((approver, index) =>
      requireNonEmpty(approver, `approval_approvers[${index}]`)
    ),
    approval_timeout_secs: assertNonNegativeInt(
      step.approval_timeout_secs,
      "approval_timeout_secs"
    ),
    compensation_job_id: optionalNonEmpty(
      step.compensation_job_id,
      "compensation_job_id"
    ),
    compensation_timeout_secs: assertNonNegativeInt(
      step.compensation_timeout_secs,
      "compensation_timeout_secs"
    ),
    concurrency_key: optionalNonEmpty(step.concurrency_key, "concurrency_key"),
    depends_on: normalizeDependsOn(step.depends_on),
    event_emit_key: optionalNonEmpty(step.event_emit_key, "event_emit_key"),
    event_key: optionalNonEmpty(step.event_key, "event_key"),
    event_notify_url: optionalNonEmpty(
      step.event_notify_url,
      "event_notify_url"
    ),
    event_timeout_secs: assertNonNegativeInt(
      step.event_timeout_secs,
      "event_timeout_secs"
    ),
    expected_duration_secs: assertNonNegativeInt(
      step.expected_duration_secs,
      "expected_duration_secs"
    ),
    job_id: optionalNonEmpty(step.job_id, "job_id"),
    max_nesting_depth: assertNonNegativeInt(
      step.max_nesting_depth,
      "max_nesting_depth"
    ),
    output_transform: optionalNonEmpty(
      step.output_transform,
      "output_transform"
    ),
    retry_initial_delay_secs: assertNonNegativeInt(
      step.retry_initial_delay_secs,
      "retry_initial_delay_secs"
    ),
    retry_max_attempts: assertNonNegativeInt(
      step.retry_max_attempts,
      "retry_max_attempts"
    ),
    retry_max_delay_secs: assertNonNegativeInt(
      step.retry_max_delay_secs,
      "retry_max_delay_secs"
    ),
    sleep_duration_secs: assertNonNegativeInt(
      step.sleep_duration_secs,
      "sleep_duration_secs"
    ),
    stage_notifications: step.stage_notifications,
    step_ref: requireNonEmpty(step.step_ref, "step_ref"),
    step_type: normalizedStepType,
    sub_workflow_id: optionalNonEmpty(step.sub_workflow_id, "sub_workflow_id"),
    timeout_secs_override: assertNonNegativeInt(
      step.timeout_secs_override,
      "timeout_secs_override"
    ),
  };

  validateStepShape(normalized);
  return normalized;
}

function validateStepShape(step: AgentWorkflowStepDefinition): void {
  const stepType = step.step_type ?? "job";

  if (stepType === "job" && step.agent_id == null && step.job_id == null) {
    throw new StraitSDKError("job steps require agent_id or job_id");
  }

  if (stepType !== "job" && step.agent_id != null) {
    throw new StraitSDKError(`${stepType} steps must not have agent_id`);
  }

  if (stepType === "approval" && (step.approval_approvers?.length ?? 0) === 0) {
    throw new StraitSDKError("approval steps require approval_approvers");
  }

  if (stepType === "sub_workflow" && step.sub_workflow_id == null) {
    throw new StraitSDKError("sub_workflow steps require sub_workflow_id");
  }

  if (stepType === "wait_for_event" && step.event_key == null) {
    throw new StraitSDKError("wait_for_event steps require event_key");
  }

  if (stepType === "sleep" && step.sleep_duration_secs == null) {
    throw new StraitSDKError("sleep steps require sleep_duration_secs");
  }
}

function collectStepRefs(steps: AgentWorkflowStepDefinition[]): Set<string> {
  const refs = new Set<string>();

  for (const step of steps) {
    if (refs.has(step.step_ref)) {
      throw new StraitSDKError(`duplicate workflow step_ref: ${step.step_ref}`);
    }
    refs.add(step.step_ref);
  }

  return refs;
}

function validateStepDependencies(
  steps: AgentWorkflowStepDefinition[],
  knownStepRefs?: Iterable<string>
): void {
  const refs = collectStepRefs(steps);
  if (knownStepRefs != null) {
    for (const stepRef of knownStepRefs) {
      refs.add(stepRef);
    }
  }

  for (const step of steps) {
    for (const dependency of step.depends_on ?? []) {
      if (!refs.has(dependency)) {
        throw new StraitSDKError(
          `unknown workflow dependency ${dependency} for step ${step.step_ref}`
        );
      }
    }
  }
}

export function agentStep(
  stepRef: string,
  agentId: string,
  options: AgentStepOptions = {}
): AgentWorkflowStepDefinition {
  return normalizeAgentWorkflowStep({
    agent_id: requireNonEmpty(agentId, "agentId"),
    compensation_job_id: optionalNonEmpty(
      options.compensationJobId,
      "compensationJobId"
    ),
    compensation_timeout_secs: assertNonNegativeInt(
      options.compensationTimeoutSecs,
      "compensationTimeoutSecs"
    ),
    depends_on: normalizeDependsOn(options.dependsOn),
    stage_notifications: options.stageNotifications,
    step_ref: requireNonEmpty(stepRef, "stepRef"),
    step_type: "job",
    ...normalizeSharedOptions(options),
  });
}

export function approvalStep(
  stepRef: string,
  options: ApprovalStepOptions = {}
): AgentWorkflowStepDefinition {
  return normalizeAgentWorkflowStep({
    approval_approvers: options.approvers?.map((approver, index) =>
      requireNonEmpty(approver, `approvers[${index}]`)
    ),
    approval_timeout_secs: assertNonNegativeInt(
      options.timeoutSecs,
      "timeoutSecs"
    ),
    depends_on: normalizeDependsOn(options.dependsOn),
    step_ref: requireNonEmpty(stepRef, "stepRef"),
    step_type: "approval",
    ...normalizeSharedOptions(options),
  });
}

export function sleepStep(
  stepRef: string,
  options: SleepStepOptions
): AgentWorkflowStepDefinition {
  return normalizeAgentWorkflowStep({
    depends_on: normalizeDependsOn(options.dependsOn),
    sleep_duration_secs: assertNonNegativeInt(
      options.durationSecs,
      "durationSecs"
    ),
    step_ref: requireNonEmpty(stepRef, "stepRef"),
    step_type: "sleep",
    ...normalizeSharedOptions(options),
  });
}

export function subWorkflowStep(
  stepRef: string,
  options: SubWorkflowStepOptions
): AgentWorkflowStepDefinition {
  return normalizeAgentWorkflowStep({
    depends_on: normalizeDependsOn(options.dependsOn),
    max_nesting_depth: assertNonNegativeInt(
      options.maxNestingDepth,
      "maxNestingDepth"
    ),
    step_ref: requireNonEmpty(stepRef, "stepRef"),
    step_type: "sub_workflow",
    sub_workflow_id: requireNonEmpty(options.subWorkflowId, "subWorkflowId"),
    ...normalizeSharedOptions(options),
  });
}

export function waitForEventStep(
  stepRef: string,
  options: WaitForEventStepOptions
): AgentWorkflowStepDefinition {
  return normalizeAgentWorkflowStep({
    depends_on: normalizeDependsOn(options.dependsOn),
    event_emit_key: optionalNonEmpty(options.eventEmitKey, "eventEmitKey"),
    event_key: requireNonEmpty(options.eventKey, "eventKey"),
    event_notify_url: optionalNonEmpty(
      options.eventNotifyUrl,
      "eventNotifyUrl"
    ),
    event_timeout_secs: assertNonNegativeInt(
      options.eventTimeoutSecs,
      "eventTimeoutSecs"
    ),
    step_ref: requireNonEmpty(stepRef, "stepRef"),
    step_type: "wait_for_event",
    ...normalizeSharedOptions(options),
  });
}

export function createDynamicSteps(
  steps: AgentWorkflowStepDefinition[],
  options: DynamicWorkflowStepOptions = {}
): DynamicWorkflowStepEnvelope {
  const maxDynamicSteps =
    options.maxDynamicSteps ?? defaultDynamicWorkflowStepLimit;
  if (!Number.isInteger(maxDynamicSteps) || maxDynamicSteps < 1) {
    throw new StraitSDKError("maxDynamicSteps must be a positive integer");
  }

  if (steps.length > maxDynamicSteps) {
    throw new StraitSDKError(
      `dynamic steps cannot contain more than ${maxDynamicSteps} steps`
    );
  }

  const normalizedSteps = steps.map(normalizeAgentWorkflowStep);
  validateStepDependencies(normalizedSteps, options.knownStepRefs);

  return Object.freeze({
    dynamic_steps: normalizedSteps,
  });
}

export function fanOutSteps(
  definition: FanOutStepsDefinition
): AgentWorkflowStepDefinition[] {
  const dependsOn = normalizeDependsOn(definition.dependsOn);
  const prefix = definition.stepRefPrefix?.trim() || "worker";

  const workerSteps = definition.workers.map((worker, index) =>
    agentStep(worker.stepRef ?? `${prefix}-${index + 1}`, worker.agentId, {
      dependsOn,
      payload: worker.payload,
    })
  );

  if (definition.synthesizer == null) {
    return workerSteps;
  }

  return [
    ...workerSteps,
    agentStep(definition.synthesizer.stepRef, definition.synthesizer.agentId, {
      dependsOn: workerSteps.map((step) => step.step_ref),
      payload: definition.synthesizer.payload,
    }),
  ];
}

export function agentWorkflow(
  definition: AgentWorkflowDefinition
): AgentWorkflowDefinition {
  const normalizedSteps = definition.steps.map(normalizeAgentWorkflowStep);
  validateStepDependencies(normalizedSteps);

  const normalized: AgentWorkflowDefinition = {
    description: definition.description,
    max_parallel_steps: definition.max_parallel_steps,
    name: requireNonEmpty(definition.name, "name"),
    project_id: requireNonEmpty(definition.project_id, "project_id"),
    slug: requireNonEmpty(definition.slug, "slug"),
    steps: normalizedSteps,
  };

  return Object.freeze(normalized);
}

export function pipelinePattern(
  definition: PipelinePatternDefinition
): AgentWorkflowDefinition {
  let previousStepRef: string | undefined;
  const steps = definition.steps.map((step) => {
    const dependsOn = previousStepRef == null ? [] : [previousStepRef];
    previousStepRef = step.stepRef;

    if (step.type === "approval") {
      return approvalStep(step.stepRef, {
        approvers: step.approvers,
        dependsOn,
        timeoutSecs: step.timeoutSecs,
      });
    }

    return agentStep(
      step.stepRef,
      requireNonEmpty(step.agentId ?? "", "agentId"),
      {
        dependsOn,
        payload: step.payload,
      }
    );
  });

  return agentWorkflow({
    description: definition.description,
    name: definition.name,
    project_id: definition.projectId,
    slug: definition.slug,
    steps,
  });
}

export function debatePattern(
  definition: DebatePatternDefinition
): AgentWorkflowDefinition {
  const analystSteps = definition.analysts.map((analyst) =>
    agentStep(analyst.stepRef, analyst.agentId, { payload: analyst.payload })
  );

  return agentWorkflow({
    description: definition.description,
    max_parallel_steps: analystSteps.length,
    name: definition.name,
    project_id: definition.projectId,
    slug: definition.slug,
    steps: [
      ...analystSteps,
      agentStep(definition.judge.stepRef, definition.judge.agentId, {
        dependsOn: analystSteps.map((step) => step.step_ref),
        payload: definition.judge.payload,
      }),
    ],
  });
}

export function orchestratorPattern(
  definition: OrchestratorPatternDefinition
): AgentWorkflowDefinition {
  const planner = agentStep(
    definition.planner.stepRef,
    definition.planner.agentId,
    {
      payload: definition.planner.payload,
    }
  );

  const fanOut = fanOutSteps({
    dependsOn: [planner.step_ref],
    synthesizer: {
      agentId: definition.synthesizer.agentId,
      payload: definition.synthesizer.payload,
      stepRef: definition.synthesizer.stepRef,
    },
    workers: definition.workers.map((worker) => ({
      agentId: worker.agentId,
      payload: worker.payload,
      stepRef: worker.stepRef,
    })),
  });

  return agentWorkflow({
    description: definition.description,
    max_parallel_steps: definition.workers.length,
    name: definition.name,
    project_id: definition.projectId,
    slug: definition.slug,
    steps: [planner, ...fanOut],
  });
}
