import type { StraitContext } from "./context";
import { normalizeBudgetInput } from "./internal";
import type { AgentBudget, JsonValue } from "./types";
import {
  agentStep,
  agentWorkflow,
  approvalStep,
  createDynamicSteps,
  debatePattern,
  fanOutSteps,
  orchestratorPattern,
  pipelinePattern,
  sleepStep,
  subWorkflowStep,
  waitForEventStep,
} from "./workflow";

export interface StraitAgentDefinition<
  TInput = JsonValue,
  TResult = JsonValue,
> {
  budget?: AgentBudget | number | string;
  description?: string;
  handler?: (context: StraitContext, input: TInput) => Promise<TResult>;
  model: string;
  name: string;
  run?: (context: StraitContext, input: TInput) => Promise<TResult>;
  slug?: string;
}

export function agent<TInput = JsonValue, TResult = JsonValue>(
  definition: StraitAgentDefinition<TInput, TResult>
): StraitAgentDefinition<TInput, TResult> {
  const run = definition.run ?? definition.handler;
  if (run == null) {
    throw new Error("agent definition requires run or handler");
  }

  return Object.freeze({
    ...definition,
    budget: normalizeBudgetInput(definition.budget),
    run,
    handler: run,
  });
}

export const strait = {
  agent,
  agentStep,
  agentWorkflow,
  approvalStep,
  createDynamicSteps,
  debatePattern,
  fanOutSteps,
  orchestratorPattern,
  pipelinePattern,
  sleepStep,
  subWorkflowStep,
  waitForEventStep,
};
