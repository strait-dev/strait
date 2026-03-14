import { Schema } from "effect";

import type { JsonSchemaLike, SchemaAdapter } from "../authoring/types";

/**
 * Adapts an Effect Schema into the authoring DSL `SchemaAdapter` contract.
 */
export const effectSchema = <TInput>(
  schema: Schema.Schema<TInput>,
  options?: { readonly toJsonSchema?: () => JsonSchemaLike | undefined }
): SchemaAdapter<TInput> => ({
  kind: "effect",
  parse: async (input) => Schema.decodeUnknownSync(schema)(input),
  toJsonSchema: options?.toJsonSchema,
});
