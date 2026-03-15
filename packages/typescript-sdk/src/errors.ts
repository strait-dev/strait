import { Data } from "effect";

export class TransportError extends Data.TaggedError("TransportError")<{
  readonly message: string;
  readonly cause?: unknown;
}> {}

export class DecodeError extends Data.TaggedError("DecodeError")<{
  readonly message: string;
  readonly body?: unknown;
  readonly cause?: unknown;
}> {}

export class ValidationError extends Data.TaggedError("ValidationError")<{
  readonly message: string;
  readonly issues?: readonly string[];
}> {}

export class UnauthorizedError extends Data.TaggedError("UnauthorizedError")<{
  readonly status: 401 | 403;
  readonly message: string;
  readonly body?: unknown;
}> {}

export class ConflictError extends Data.TaggedError("ConflictError")<{
  readonly status: 409;
  readonly message: string;
  readonly body?: unknown;
}> {}

export class NotFoundError extends Data.TaggedError("NotFoundError")<{
  readonly status: 404;
  readonly message: string;
  readonly body?: unknown;
}> {}

export class RateLimitedError extends Data.TaggedError("RateLimitedError")<{
  readonly status: 429;
  readonly message: string;
  readonly body?: unknown;
}> {}

export class ApiError extends Data.TaggedError("ApiError")<{
  readonly status: number;
  readonly message: string;
  readonly body?: unknown;
}> {}

/**
 * Thrown when a polling operation (e.g. {@link waitForRun}) exceeds its timeout.
 *
 * @example
 * ```ts
 * try {
 *   await waitForRun(getRun, runId, { timeoutMs: 5000 });
 * } catch (e) {
 *   if (e instanceof TimeoutError) {
 *     console.log(`Run ${e.runId} timed out after ${e.elapsedMs}ms`);
 *   }
 * }
 * ```
 */
export class TimeoutError extends Data.TaggedError("TimeoutError")<{
  readonly message: string;
  readonly runId: string;
  readonly elapsedMs: number;
}> {}

/**
 * Thrown when workflow step DAG validation fails at definition time.
 *
 * Contains details about which specific validation rules were violated.
 */
export class DagValidationError extends Data.TaggedError("DagValidationError")<{
  readonly message: string;
  readonly cycles?: readonly string[];
  readonly missingRefs?: readonly string[];
  readonly duplicateRefs?: readonly string[];
}> {}

export type StraitSdkError =
  | ApiError
  | ConflictError
  | DagValidationError
  | DecodeError
  | NotFoundError
  | RateLimitedError
  | TimeoutError
  | TransportError
  | UnauthorizedError
  | ValidationError;

export const mapHttpError = (
  status: number,
  message: string,
  body?: unknown
): StraitSdkError => {
  if (status === 401 || status === 403) {
    return new UnauthorizedError({ status, message, body });
  }

  if (status === 404) {
    return new NotFoundError({ status, message, body });
  }

  if (status === 409) {
    return new ConflictError({ status, message, body });
  }

  if (status === 429) {
    return new RateLimitedError({ status, message, body });
  }

  return new ApiError({ status, message, body });
};
