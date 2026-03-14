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
