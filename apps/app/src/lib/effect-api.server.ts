import { Data, Effect } from "effect";
import { apiRequest, type RequestOptions } from "@/lib/api-client.server";
import { captureException } from "@/lib/sentry";

export class ApiError extends Data.TaggedError("ApiError")<{
  readonly path: string;
  readonly method: string;
  readonly cause: unknown;
}> {}

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
