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
  name: string;
  timeoutMs?: number;
}

function requireName(value: string, field: string): string {
  const normalized = value.trim();
  if (normalized.length === 0) {
    throw new StraitSDKError(`${field} is required`);
  }
  return normalized;
}

export function createSandboxTool<TInput = JsonValue, TResult = JsonValue>(
  options: CreateSandboxToolOptions<TInput, TResult>
): SandboxTool<TInput, TResult> {
  return Object.freeze({
    name: requireName(options.name, "name"),
    description: options.description?.trim() || undefined,
    sandbox: {
      mode: "dynamic-worker" as const,
      image: options.image?.trim() || undefined,
      timeoutMs: options.timeoutMs,
    },
    execute: (input: TInput) =>
      runPromise(
        Effect.tryPromise({
          try: () => Promise.resolve(options.execute(input)),
          catch: (error) =>
            error instanceof Error ? error : new Error(String(error)),
        })
      ),
  });
}
