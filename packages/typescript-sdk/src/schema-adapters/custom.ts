import type { JsonSchemaLike, SchemaAdapter } from "../authoring/types";

/**
 * Creates a schema adapter from a custom parse function.
 *
 * Use this when your validation library isn't Zod or Effect Schema, or when
 * you want full control over parsing and JSON schema generation.
 *
 * Works with any validator: Joi, Yup, Valibot, Superstruct, ArkType,
 * or plain functions.
 *
 * @param parse - Validation function that accepts unknown input and returns
 *   the parsed/validated value. May throw on invalid input.
 * @param options - Optional configuration.
 * @param options.toJsonSchema - Function returning a JSON schema representation
 *   for API registration. If omitted, no schema is sent to the API.
 *
 * @example
 * ```ts
 * // Plain validation function
 * const schema = customSchema<{ sku: string }>((input) => {
 *   if (typeof input !== "object" || input === null || !("sku" in input)) {
 *     throw new Error("Invalid payload: missing sku");
 *   }
 *   return input as { sku: string };
 * });
 *
 * // With Valibot
 * import * as v from "valibot";
 * const valibotSchema = v.object({ sku: v.string() });
 * const schema = customSchema<{ sku: string }>(
 *   (input) => v.parse(valibotSchema, input),
 * );
 *
 * // With Yup
 * import * as yup from "yup";
 * const yupSchema = yup.object({ sku: yup.string().required() });
 * const schema = customSchema<{ sku: string }>(
 *   (input) => yupSchema.validateSync(input),
 * );
 *
 * // With Joi
 * import Joi from "joi";
 * const joiSchema = Joi.object({ sku: Joi.string().required() });
 * const schema = customSchema<{ sku: string }>(
 *   (input) => {
 *     const { value, error } = joiSchema.validate(input);
 *     if (error) throw error;
 *     return value;
 *   },
 * );
 * ```
 */
export const customSchema = <TInput>(
  parse: (input: unknown) => TInput | Promise<TInput>,
  options?: {
    readonly toJsonSchema?: () => JsonSchemaLike | undefined;
  }
): SchemaAdapter<TInput> => ({
  kind: "custom",
  parse: async (input) => parse(input),
  toJsonSchema: options?.toJsonSchema,
});
