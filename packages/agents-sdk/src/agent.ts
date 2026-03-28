import type { StraitContext } from "./context";
import type { AgentBudget, JsonValue } from "./types";
import {
  agentWorkflow,
  agentStep,
  approvalStep,
  debatePattern,
  orchestratorPattern,
  pipelinePattern,
} from "./workflow";

export interface StraitAgentDefinition<TInput = JsonValue, TResult = JsonValue> {
  name: string;
  slug?: string;
  description?: string;
  model: string;
  budget?: AgentBudget;
  run: (context: StraitContext, input: TInput) => Promise<TResult>;
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
