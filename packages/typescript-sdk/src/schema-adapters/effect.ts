import { Schema } from "effect";

import type { JsonSchemaLike, SchemaAdapter } from "../authoring/types";

export const effectSchema = <TInput>(
  schema: Schema.Schema<TInput>,
  options?: { readonly toJsonSchema?: () => JsonSchemaLike | undefined }
): SchemaAdapter<TInput> => ({
  kind: "effect",
  parse: async (input) => Schema.decodeUnknownSync(schema)(input),
  toJsonSchema: options?.toJsonSchema,
});
