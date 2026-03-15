import type { FutureLocalExecutorHooks, SchemaAdapter } from "./types";
import { defineWorkflow, type DefineWorkflowOptions } from "./workflow";

type DefineDagOptions<TPayload> = Omit<DefineWorkflowOptions<TPayload>, "run" | "onSuccess" | "onFailure">;

/**
 * Defines a DAG-flavored workflow authoring unit.
 *
 * DAG definitions reuse workflow registration/trigger mechanics while branding the
 * local definition and hook context as `dag`.
 */
export const defineDag = <TPayload>(options: DefineDagOptions<TPayload>) => {
  const workflowDefinition = defineWorkflow({
    ...options,
    hooks: {
      ...options.hooks,
      onRegister: async (context) => {
        await options.hooks?.onRegister?.({
          ...context,
          kind: "dag",
        });
      },
    },
  });

  return {
    ...workflowDefinition,
    kind: "dag" as const,
  };
};
