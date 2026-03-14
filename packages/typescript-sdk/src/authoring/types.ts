import type { SdkResult } from "../composition/result";

/**
 * Minimal JSON Schema shape accepted by authoring helpers.
 */
export type JsonSchemaLike = Readonly<Record<string, unknown>>;

/**
 * Runtime validation adapter used by authoring DSL helpers.
 */
export type SchemaAdapter<TInput> = {
  /** Adapter family identifier (for diagnostics/introspection). */
  readonly kind: string;
  /** Parses and validates unknown payloads before trigger calls. */
  readonly parse: (input: unknown) => Promise<TInput>;
  /** Optional JSON schema projection used for registration payloads. */
  readonly toJsonSchema?: () => JsonSchemaLike | undefined;
};

/**
 * Promise wrapper used by `triggerResult` helpers.
 */
export type TriggerResult<TOutput> = Promise<SdkResult<TOutput, unknown>>;

/**
 * Extension points for future local execution integrations.
 */
export type FutureLocalExecutorHooks = {
  /** Invoked after successful remote registration. */
  readonly onRegister?: (context: {
    readonly kind: "dag" | "job" | "workflow";
    readonly slug: string;
    readonly registration: Readonly<Record<string, unknown>>;
  }) => void | Promise<void>;
};
