import type { StraitContext } from "./context";
import type { AgentBudget, JsonValue } from "./types";
import {
  agentStep,
  agentWorkflow,
  approvalStep,
  debatePattern,
  orchestratorPattern,
  pipelinePattern,
} from "./workflow";

export interface StraitAgentDefinition<
  TInput = JsonValue,
  TResult = JsonValue,
> {
  budget?: AgentBudget;
  description?: string;
  model: string;
  name: string;
  run: (context: StraitContext, input: TInput) => Promise<TResult>;
  slug?: string;
}

export function agent<TInput = JsonValue, TResult = JsonValue>(
  definition: StraitAgentDefinition<TInput, TResult>
): StraitAgentDefinition<TInput, TResult> {
  return Object.freeze({ ...definition });
}

export const strait = {
  agent,
  agentStep,
  agentWorkflow,
  approvalStep,
  debatePattern,
  orchestratorPattern,
  pipelinePattern,
};
