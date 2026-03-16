import type { StandardSchemaV1 } from "@standard-schema/spec";

import type { SchemaAdapter } from "../authoring/types";

/**
 * Error thrown when a Standard Schema validation fails.
 *
 * Contains the structured issues from the schema's `~standard.validate` result.
 */
export class StandardSchemaValidationError extends Error {
  readonly issues: readonly StandardSchemaV1.Issue[];

  constructor(issues: readonly StandardSchemaV1.Issue[]) {
    const message = issues.map((i) => i.message).join("; ");
    super(`Validation failed: ${message}`);
    this.name = "StandardSchemaValidationError";
    this.issues = issues;
  }
}

/**
 * Checks whether a value implements the Standard Schema v1 interface.
 *
 * Use this to detect if a schema from Zod, Valibot, ArkType, or any other
 * Standard Schema compliant library can be used directly.
 *
 * @param value - The value to check.
 * @returns `true` if the value has a `~standard` property with `version: 1` and a `validate` function.
 *
 * @example
 * ```ts
 * import { z } from "zod";
 * import * as v from "valibot";
 *
 * isStandardSchema(z.string());      // true (Zod 3.24+)
 * isStandardSchema(v.string());      // true (Valibot 1.0+)
 * isStandardSchema({ parse: ... });  // false (not standard schema)
 * ```
 */
export const isStandardSchema = (value: unknown): value is StandardSchemaV1 =>
  typeof value === "object" &&
  value !== null &&
  "~standard" in value &&
  typeof (value as Record<string, unknown>)["~standard"] === "object" &&
  (value as Record<string, Record<string, unknown>>)["~standard"] !== null &&
  (value as Record<string, Record<string, unknown>>)["~standard"].version ===
    1 &&
  typeof (value as Record<string, Record<string, unknown>>)["~standard"]
    .validate === "function";

/**
 * Adapts any Standard Schema v1 compliant schema into the SDK's
 * {@link SchemaAdapter} contract.
 *
 * Works with any library that implements the
 * [Standard Schema](https://github.com/standard-schema/standard-schema)
 * specification including:
 * - **Zod** (v3.24+)
 * - **Valibot** (v1.0+)
 * - **ArkType** (v2.0+)
 * - **Effect Schema** (v3.10+)
 * - Any custom implementation
 *
 * @param schema - A Standard Schema v1 compliant schema object.
 * @returns A {@link SchemaAdapter} that validates payloads using the schema's
 *   `~standard.validate` method.
 * @throws {StandardSchemaValidationError} When validation fails, with structured issues.
 *
 * @example
 * ```ts
 * import { z } from "zod";
 * import * as v from "valibot";
 * import { type } from "arktype";
 *
 * // With Zod
 * const job1 = defineJob({
 *   schema: standardSchema(z.object({ sku: z.string() })),
 *   // ...
 * });
 *
 * // With Valibot
 * const job2 = defineJob({
 *   schema: standardSchema(v.object({ sku: v.string() })),
 *   // ...
 * });
 *
 * // With ArkType
 * const job3 = defineJob({
 *   schema: standardSchema(type({ sku: "string" })),
 *   // ...
 * });
 * ```
 */
export const standardSchema = <TInput, TOutput = TInput>(
  schema: StandardSchemaV1<TInput, TOutput>
): SchemaAdapter<TOutput> => ({
  kind: `standard:${schema["~standard"].vendor}`,
  parse: async (input) => {
    let result = schema["~standard"].validate(input);
    if (result instanceof Promise) {
      result = await result;
    }

    if (result.issues) {
      throw new StandardSchemaValidationError(result.issues);
    }

    return result.value;
  },
});
