import type { StandardSchemaV1 } from "@standard-schema/spec";

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
 * Accepted schema input for authoring DSL helpers.
 *
 * You can pass either:
 * - A {@link SchemaAdapter} created via `zodSchema()`, `effectSchema()`,
 *   `customSchema()`, or `standardSchema()`
 * - A raw Standard Schema v1 compliant object (Zod 3.24+, Valibot 1.0+,
 *   ArkType 2.0+, etc.) — it will be auto-wrapped
 *
 * @example
 * ```ts
 * import { z } from "zod";
 * import * as v from "valibot";
 *
 * // All of these work in defineJob({ schema: ... }):
 * defineJob({ schema: z.object({ sku: z.string() }), ... });       // Zod (auto-detected)
 * defineJob({ schema: v.object({ sku: v.string() }), ... });       // Valibot (auto-detected)
 * defineJob({ schema: zodSchema(myZod), ... });                     // Explicit adapter
 * defineJob({ schema: standardSchema(mySchema), ... });             // Explicit adapter
 * defineJob({ schema: customSchema((i) => validate(i)), ... });     // Custom
 * ```
 */
export type SchemaInput<TInput> =
  | SchemaAdapter<TInput>
  | StandardSchemaV1<TInput, TInput>;

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
