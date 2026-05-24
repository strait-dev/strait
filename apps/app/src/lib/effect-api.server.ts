import { Data, Effect, Schema } from "effect";
import {
  apiClientErrorToError,
  apiRequestEffect,
  type RequestOptions,
} from "@/lib/api-client.server";
import { captureException } from "@/lib/sentry";

/**
 * Tagged error type for Go API failures within the Effect pipeline.
 *
 * Wraps the original thrown error (`cause`) together with the request
 * `path` and `method` so that downstream error handlers (Sentry reporters,
 * fallback runners) have structured context without parsing error messages.
 */
export class ApiError extends Data.TaggedError("ApiError")<{
  readonly path: string;
  readonly method: string;
  readonly cause: unknown;
}> {}

function reportableCause(cause: unknown): unknown {
  if (
    cause &&
    typeof cause === "object" &&
    "_tag" in cause &&
    cause._tag === "ApiClientError"
  ) {
    return apiClientErrorToError(
      cause as Parameters<typeof apiClientErrorToError>[0]
    );
  }
  return cause;
}

/**
 * Adds call-site context to a typed Go API client Effect.
 *
 * Use this as the building block for all Go API calls inside server
 * functions. Pair with `runWithFallback` (silent fallback) or
 * `runWithSentryReport` (re-throw for React Query error boundaries).
 *
 * @param path   - API path, e.g. `"/v1/jobs"`.
 * @param options - Optional method, body, and query params.
 */
export function apiEffect<T>(
  path: string,
  options: RequestOptions = {}
): Effect.Effect<T, ApiError> {
  return apiRequestEffect<T>(path, options).pipe(
    Effect.mapError(
      (cause) => new ApiError({ path, method: options.method ?? "GET", cause })
    )
  );
}

/**
 * Lifts an `apiRequest` call into an Effect and validates the response
 * against an Effect Schema, converting parse failures into typed `ApiError` values.
 *
 * Use this when you want runtime validation that the Go API response
 * matches the expected shape. Parse errors are wrapped as `ApiError`
 * so they flow through the same error channel as network failures.
 *
 * @param path    - API path, e.g. `"/v1/usage/current"`.
 * @param schema  - Effect Schema to validate the response against.
 * @param options - Optional method, body, and query params.
 */
export const apiEffectWithSchema = <A, I>(
  path: string,
  schema: Schema.Schema<A, I>,
  options: RequestOptions = {}
): Effect.Effect<A, ApiError> =>
  apiEffect<unknown>(path, options).pipe(
    Effect.flatMap((data) =>
      Schema.decodeUnknown(schema)(data).pipe(
        Effect.mapError(
          (parseError) =>
            new ApiError({
              path,
              method: options.method ?? "GET",
              cause: parseError,
            })
        )
      )
    )
  );

/**
 * Runs an Effect pipeline, reports any error to Sentry, and returns
 * the provided `fallback` value on failure.
 *
 * Best for fire-and-forget or best-effort calls (e.g. syncing a
 * project to the Go service) where the caller already has a primary
 * result and the API call is supplementary.
 *
 * @param effect   - The Effect to execute.
 * @param fallback - Value returned when the effect fails.
 */
export function runWithFallback<T>(
  effect: Effect.Effect<T, ApiError>,
  fallback: T
): Promise<T> {
  return Effect.runPromise(
    effect.pipe(
      Effect.tapError((error) =>
        Effect.sync(() =>
          captureException(reportableCause(error.cause), {
            tags: {
              location: "server_function",
              api_path: error.path,
              api_method: error.method,
            },
          })
        )
      ),
      Effect.catchAll(() => Effect.succeed(fallback))
    )
  );
}

/**
 * Runs an Effect pipeline, reports any error to Sentry, then re-throws
 * the original error so React Query receives it naturally.
 *
 * Use this for API hooks where query/mutation error states should be
 * surfaced to the UI via React Query error boundaries.
 *
 * @param effect - The Effect to execute.
 */
export function runWithSentryReport<T>(
  effect: Effect.Effect<T, ApiError>
): Promise<T> {
  return Effect.runPromise(
    effect.pipe(
      Effect.tapError((error) =>
        Effect.sync(() =>
          captureException(reportableCause(error.cause), {
            tags: {
              location: "server_function",
              api_path: error.path,
              api_method: error.method,
            },
          })
        )
      ),
      Effect.catchAll((error) => Effect.die(reportableCause(error.cause)))
    )
  );
}
