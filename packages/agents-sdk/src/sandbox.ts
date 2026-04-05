import { Effect } from "effect";

import { runPromise } from "./effects";
import { StraitSDKError } from "./errors";
import type { JsonValue, SandboxTool } from "./types";

export interface CreateSandboxToolOptions<
  TInput = JsonValue,
  TResult = JsonValue,
> {
  description?: string;
  execute: (input: TInput) => Promise<TResult> | TResult;
  image?: string;
  mode?: "dynamic-worker" | "outbound-worker";
  name: string;
  networkClass?: string;
  outboundPolicyTag?: string;
  outputSchema?: JsonSchemaObject;
  runtime?: string;
  timeoutMs?: number;
}

/**
 * Minimal JSON Schema object for tool output validation.
 * Supports: type, properties, required, items, enum.
 */
export interface JsonSchemaObject {
  enum?: readonly JsonValue[];
  items?: JsonSchemaObject;
  properties?: Record<string, JsonSchemaObject>;
  required?: readonly string[];
  type?: string;
}

function requireName(value: string, field: string): string {
  const normalized = value.trim();
  if (normalized.length === 0) {
    throw new StraitSDKError(`${field} is required`);
  }
  return normalized;
}

/**
 * Validates a value against a minimal JSON Schema subset.
 * Supports: type, properties, required, items, enum.
 */
// biome-ignore lint/complexity/noExcessiveCognitiveComplexity: validation logic is inherently branchy
export function validateJsonSchema(
  value: unknown,
  schema: JsonSchemaObject
): string | null {
  if (schema.type != null) {
    const actual = Array.isArray(value) ? "array" : typeof value;
    if (schema.type === "array" && !Array.isArray(value)) {
      return `expected array, got ${actual}`;
    }
    if (
      schema.type === "object" &&
      (typeof value !== "object" || value === null || Array.isArray(value))
    ) {
      return `expected object, got ${actual}`;
    }
    if (schema.type === "string" && typeof value !== "string") {
      return `expected string, got ${actual}`;
    }
    if (schema.type === "number" && typeof value !== "number") {
      return `expected number, got ${actual}`;
    }
    if (schema.type === "boolean" && typeof value !== "boolean") {
      return `expected boolean, got ${actual}`;
    }
  }

  if (schema.enum != null) {
    const serialized = JSON.stringify(value);
    if (!schema.enum.some((e) => JSON.stringify(e) === serialized)) {
      return `value not in enum: ${JSON.stringify(schema.enum)}`;
    }
  }

  if (
    schema.required != null &&
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value)
  ) {
    const obj = value as Record<string, unknown>;
    for (const key of schema.required) {
      if (!(key in obj)) {
        return `missing required property: ${key}`;
      }
    }
  }

  if (
    schema.properties != null &&
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value)
  ) {
    const obj = value as Record<string, unknown>;
    for (const [key, propSchema] of Object.entries(schema.properties)) {
      if (key in obj) {
        const error = validateJsonSchema(obj[key], propSchema);
        if (error != null) {
          return `property "${key}": ${error}`;
        }
      }
    }
  }

  if (schema.items != null && Array.isArray(value)) {
    for (let i = 0; i < value.length; i++) {
      const error = validateJsonSchema(value[i], schema.items);
      if (error != null) {
        return `item[${i}]: ${error}`;
      }
    }
  }

  return null;
}

export function createSandboxTool<TInput = JsonValue, TResult = JsonValue>(
  options: CreateSandboxToolOptions<TInput, TResult>
): SandboxTool<TInput, TResult> {
  const outputSchema = options.outputSchema;

  return Object.freeze({
    name: requireName(options.name, "name"),
    description: options.description?.trim() || undefined,
    sandbox: {
      executionMode: "sandboxed" as const,
      mode: options.mode ?? ("dynamic-worker" as const),
      image: options.image?.trim() || undefined,
      networkClass: options.networkClass?.trim() || undefined,
      outboundPolicyTag: options.outboundPolicyTag?.trim() || undefined,
      runtime: options.runtime?.trim() || undefined,
      timeoutMs: options.timeoutMs,
    },
    execute: (input: TInput) =>
      runPromise(
        Effect.tryPromise({
          try: async () => {
            const result = await Promise.resolve(options.execute(input));
            if (outputSchema != null) {
              const error = validateJsonSchema(result, outputSchema);
              if (error != null) {
                throw new StraitSDKError(
                  `Tool "${options.name}" output validation failed: ${error}`
                );
              }
            }
            return result;
          },
          catch: (error) =>
            error instanceof Error ? error : new Error(String(error)),
        })
      ),
  });
}
