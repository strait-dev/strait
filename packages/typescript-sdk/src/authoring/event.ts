import type { SchemaAdapter, SchemaInput } from "./types";
import { resolveSchema } from "./utils";

export type EventDefinition<TPayload> = {
  readonly key: string;
  readonly schema: SchemaAdapter<TPayload>;
  readonly parse: (input: unknown) => Promise<TPayload>;
};

export const defineEvent = <TPayload>(
  key: string,
  schema: SchemaInput<TPayload>
): EventDefinition<TPayload> => {
  const resolved = resolveSchema(schema);
  return {
    key,
    schema: resolved,
    parse: (input: unknown) => resolved.parse(input),
  };
};
