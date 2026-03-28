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

export interface AgentWorkflowStepDefinition {
  agent_id?: string;
  approval_approvers?: string[];
  approval_timeout_secs?: number;
  depends_on?: string[];
  job_id?: string;
  on_failure?: WorkflowFailurePolicy;
  payload?: JsonValue;
  step_ref: string;
  step_type?: WorkflowStepType;
  timeout_secs_override?: number;
}

export interface AgentWorkflowDefinition {
  description?: string;
  max_parallel_steps?: number;
  name: string;
  project_id: string;
  slug: string;
  steps: AgentWorkflowStepDefinition[];
}

export interface AgentStepOptions {
  dependsOn?: string[];
  onFailure?: WorkflowFailurePolicy;
  payload?: JsonValue;
  timeoutSecsOverride?: number;
}

export interface ApprovalStepOptions {
  approvers?: string[];
  dependsOn?: string[];
  onFailure?: WorkflowFailurePolicy;
  timeoutSecs?: number;
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

function requireNonEmpty(value: string, field: string): string {
  const normalized = value.trim();
  if (normalized.length === 0) {
    throw new StraitSDKError(`${field} is required`);
  }
  return normalized;
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

function validateUniqueStepRefs(steps: AgentWorkflowStepDefinition[]): void {
  const refs = new Set<string>();
  for (const step of steps) {
    if (refs.has(step.step_ref)) {
      throw new StraitSDKError(`duplicate workflow step_ref: ${step.step_ref}`);
    }
    refs.add(step.step_ref);
  }
}

export function agentStep(
  stepRef: string,
  agentId: string,
  options: AgentStepOptions = {}
): AgentWorkflowStepDefinition {
  return {
    agent_id: requireNonEmpty(agentId, "agentId"),
    depends_on: normalizeDependsOn(options.dependsOn),
    on_failure: options.onFailure,
    payload: options.payload,
    step_ref: requireNonEmpty(stepRef, "stepRef"),
    step_type: "job",
    timeout_secs_override: options.timeoutSecsOverride,
  };
}

export function approvalStep(
  stepRef: string,
  options: ApprovalStepOptions = {}
): AgentWorkflowStepDefinition {
  return {
    approval_approvers: options.approvers?.map((approver, index) =>
      requireNonEmpty(approver, `approvers[${index}]`)
    ),
    approval_timeout_secs: options.timeoutSecs,
    depends_on: normalizeDependsOn(options.dependsOn),
    on_failure: options.onFailure,
    step_ref: requireNonEmpty(stepRef, "stepRef"),
    step_type: "approval",
  };
}

export function agentWorkflow(
  definition: AgentWorkflowDefinition
): AgentWorkflowDefinition {
  const normalized: AgentWorkflowDefinition = {
    description: definition.description,
    max_parallel_steps: definition.max_parallel_steps,
    name: requireNonEmpty(definition.name, "name"),
    project_id: requireNonEmpty(definition.project_id, "project_id"),
    slug: requireNonEmpty(definition.slug, "slug"),
    steps: definition.steps.map((step) => ({
      ...step,
      depends_on: normalizeDependsOn(step.depends_on),
      step_ref: requireNonEmpty(step.step_ref, "step_ref"),
    })),
  };

  validateUniqueStepRefs(normalized.steps);
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
  const judgeDependsOn = analystSteps.map((step) => step.step_ref);

  return agentWorkflow({
    description: definition.description,
    max_parallel_steps: analystSteps.length,
    name: definition.name,
    project_id: definition.projectId,
    slug: definition.slug,
    steps: [
      ...analystSteps,
      agentStep(definition.judge.stepRef, definition.judge.agentId, {
        dependsOn: judgeDependsOn,
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
  const workers = definition.workers.map((worker) =>
    agentStep(worker.stepRef, worker.agentId, {
      dependsOn: [planner.step_ref],
      payload: worker.payload,
    })
  );

  return agentWorkflow({
    description: definition.description,
    max_parallel_steps: workers.length,
    name: definition.name,
    project_id: definition.projectId,
    slug: definition.slug,
    steps: [
      planner,
      ...workers,
      agentStep(
        definition.synthesizer.stepRef,
        definition.synthesizer.agentId,
        {
          dependsOn: workers.map((worker) => worker.step_ref),
          payload: definition.synthesizer.payload,
        }
      ),
    ],
  });
}
