import type { JsonSchemaLike, SchemaAdapter } from "../authoring/types";

/**
 * Minimal Zod-like contract used by `zodSchema`.
 */
export type ZodSchemaLike<TInput> = {
  readonly parse: (input: unknown) => TInput;
  readonly toJSON?: () => unknown;
};

const toJsonSchema = (
  schema: ZodSchemaLike<unknown>
): JsonSchemaLike | undefined => {
  if (!schema.toJSON) {
    return undefined;
  }

  const json = schema.toJSON();
  if (typeof json !== "object" || json === null || Array.isArray(json)) {
    return undefined;
  }

  return json as JsonSchemaLike;
};

/**
 * Adapts a Zod-like schema into the authoring DSL `SchemaAdapter` contract.
 */
export const zodSchema = <TInput>(
  schema: ZodSchemaLike<TInput>
): SchemaAdapter<TInput> => ({
  kind: "zod",
  parse: async (input) => schema.parse(input),
  toJsonSchema: () => toJsonSchema(schema),
});
