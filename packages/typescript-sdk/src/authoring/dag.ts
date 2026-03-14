import type { FutureLocalExecutorHooks, SchemaAdapter } from "./types";
import { defineWorkflow } from "./workflow";

type DefineDagOptions<TPayload> = {
  readonly name: string;
  readonly slug: string;
  readonly steps: readonly Readonly<Record<string, unknown>>[];
  readonly schema: SchemaAdapter<TPayload>;
  readonly projectId?: string;
  readonly description?: string;
  readonly defaults?: Readonly<Record<string, unknown>>;
  readonly hooks?: FutureLocalExecutorHooks;
};

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
