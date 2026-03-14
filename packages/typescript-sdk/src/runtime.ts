import { Context, Effect, Layer, Schema } from "effect";

import {
  normalizeBaseUrl,
  type StraitClientConfig,
  StraitClientConfigSchema,
} from "./config";
import { ValidationError } from "./errors";

export type FetchLike = (
  input: RequestInfo | URL,
  init?: RequestInit
) => Promise<Response>;

export type StraitRuntime = {
  readonly config: StraitClientConfig;
  readonly fetch: FetchLike;
};

export class StraitRuntimeTag extends Context.Tag("StraitRuntime")<
  StraitRuntimeTag,
  StraitRuntime
>() {}

const decodeConfig = Schema.decodeUnknown(StraitClientConfigSchema);

export const createRuntime = (
  input: unknown,
  options?: {
    readonly fetch?: FetchLike;
  }
): Effect.Effect<StraitRuntime, ValidationError, never> =>
  decodeConfig(input).pipe(
    Effect.map((decoded) => ({
      config: {
        ...decoded,
        baseUrl: normalizeBaseUrl(decoded.baseUrl),
      },
      fetch: options?.fetch ?? globalThis.fetch,
    })),
    Effect.mapError(
      (cause) =>
        new ValidationError({
          message: "invalid SDK configuration",
          issues: [String(cause)],
        })
    )
  );

export const runtimeLayer = (
  input: unknown,
  options?: {
    readonly fetch?: FetchLike;
  }
): Layer.Layer<StraitRuntimeTag, ValidationError, never> =>
  Layer.effect(StraitRuntimeTag, createRuntime(input, options));

export const provideRuntime = <A, E, R>(
  effect: Effect.Effect<A, E, R | StraitRuntimeTag>,
  input: unknown,
  options?: {
    readonly fetch?: FetchLike;
  }
): Effect.Effect<A, E | ValidationError, Exclude<R, StraitRuntimeTag>> =>
  Effect.provide(effect, runtimeLayer(input, options));
