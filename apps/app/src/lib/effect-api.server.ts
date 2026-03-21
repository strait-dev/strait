import { Data, Effect } from "effect";
import { apiRequest, type RequestOptions } from "@/lib/api-client.server";
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

/**
 * Lifts an `apiRequest` call into an Effect, converting thrown errors
 * into typed `ApiError` values.
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
  return Effect.tryPromise({
    try: () => apiRequest<T>(path, options),
    catch: (cause) =>
      new ApiError({ path, method: options.method ?? "GET", cause }),
  });
}

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
          captureException(error.cause, {
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
          captureException(error.cause, {
            tags: {
              location: "server_function",
              api_path: error.path,
              api_method: error.method,
            },
          })
        )
      ),
      Effect.catchAll((error) => Effect.die(error.cause))
    )
  );
}
