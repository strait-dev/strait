import type { SdkResult } from "../composition/result";

export type JsonSchemaLike = Readonly<Record<string, unknown>>;

export type SchemaAdapter<TInput> = {
  readonly kind: string;
  readonly parse: (input: unknown) => Promise<TInput>;
  readonly toJsonSchema?: () => JsonSchemaLike | undefined;
};

export type TriggerResult<TOutput> = Promise<SdkResult<TOutput, unknown>>;

export type FutureLocalExecutorHooks = {
  readonly onRegister?: (context: {
    readonly kind: "dag" | "job" | "workflow";
    readonly slug: string;
    readonly registration: Readonly<Record<string, unknown>>;
  }) => void | Promise<void>;
};
