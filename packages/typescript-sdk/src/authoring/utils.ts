import { isStandardSchema, standardSchema } from "../schema-adapters/standard";
import type { SchemaAdapter, SchemaInput } from "./types";

/**
 * Resolves a {@link SchemaInput} to a {@link SchemaAdapter}.
 *
 * If the input is already a `SchemaAdapter` (has a `kind` and `parse` property),
 * it is returned as-is. If it implements Standard Schema v1 (has `~standard`
 * with `version: 1` and a `validate` function), it is auto-wrapped via
 * `standardSchema()`.
 *
 * @param input - Either a `SchemaAdapter` or a Standard Schema v1 compliant object.
 * @returns A `SchemaAdapter` ready for use in the authoring DSL.
 */
export const resolveSchema = <TInput>(
  input: SchemaInput<TInput>
): SchemaAdapter<TInput> => {
  // Already a SchemaAdapter
  if ("kind" in input && "parse" in input) {
    return input as SchemaAdapter<TInput>;
  }

  // Standard Schema v1
  if (isStandardSchema(input)) {
    return standardSchema(input) as SchemaAdapter<TInput>;
  }

  throw new Error(
    "Invalid schema: expected a SchemaAdapter (from zodSchema/effectSchema/customSchema/standardSchema) " +
      "or a Standard Schema v1 compliant object (Zod 3.24+, Valibot 1.0+, ArkType 2.0+)"
  );
};

/**
 * Resolves a project ID from definition-time or registration-time values.
 *
 * @param definitionProjectId - Project ID from the definition options.
 * @param registrationProjectId - Project ID from the register() call.
 * @param entityLabel - Label used in the error message (e.g. "defineJob(my-slug)").
 * @returns The resolved project ID.
 * @throws {Error} If neither project ID is provided.
 */
export const requireProjectId = (
  definitionProjectId: string | undefined,
  registrationProjectId: string | undefined,
  entityLabel: string
): string => {
  const resolved = registrationProjectId ?? definitionProjectId;
  if (!resolved) {
    throw new Error(
      `${entityLabel} requires projectId in definition or register() call`
    );
  }

  return resolved;
};

/**
 * Extracts an `id` field from an unknown API response object.
 *
 * @param value - The API response to extract from.
 * @returns The `id` string if present, otherwise `undefined`.
 */
export const extractEntityId = (value: unknown): string | undefined => {
  if (
    typeof value === "object" &&
    value !== null &&
    "id" in value &&
    typeof (value as { readonly id: unknown }).id === "string"
  ) {
    return (value as { readonly id: string }).id;
  }

  return undefined;
};
