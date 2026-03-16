import {
  type LanguageModel,
  ToolLoopAgent,
  type ToolLoopAgentSettings,
  type ToolSet,
  wrapLanguageModel,
} from "ai";
import type { RunContext } from "../authoring/job";
import { createStraitProvider } from "./provider";
import { type CreateStraitToolsOptions, createStraitTools } from "./tools";
import type { StraitProviderOptions } from "./types";

export type CreateStraitAgentOptions = {
  readonly model: LanguageModel;
  readonly instructions?: string;
  readonly tools?: ToolSet;
  readonly straitTools?: CreateStraitToolsOptions;
  readonly providerOptions?: StraitProviderOptions;
  readonly stopWhen?: ToolLoopAgentSettings["stopWhen"];
  readonly onStepFinish?: ToolLoopAgentSettings["onStepFinish"];
  readonly onFinish?: ToolLoopAgentSettings["onFinish"];
  readonly maxOutputTokens?: number;
  readonly temperature?: number;
};

export const createStraitAgent = (
  ctx: RunContext,
  options: CreateStraitAgentOptions
): ToolLoopAgent => {
  const middleware = createStraitProvider(ctx, options.providerOptions);
  const wrappedModel = wrapLanguageModel({
    model: options.model as Parameters<typeof wrapLanguageModel>[0]["model"],
    middleware,
  });

  const straitTools = createStraitTools(ctx, options.straitTools);
  const allTools = { ...straitTools, ...options.tools };

  return new ToolLoopAgent({
    model: wrappedModel,
    instructions: options.instructions,
    tools: allTools,
    stopWhen: options.stopWhen,
    onStepFinish: options.onStepFinish,
    onFinish: options.onFinish,
    maxOutputTokens: options.maxOutputTokens,
    temperature: options.temperature,
  });
};
