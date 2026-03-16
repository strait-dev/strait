/**
 * Retry policy for {@link withRetry}.
 */
export type RetryOptions<TError = unknown> = {
  /** Number of total attempts including the first call. */
  readonly attempts?: number;
  /** Initial retry delay in milliseconds. */
  readonly delayMs?: number;
  /** Exponential backoff multiplier applied after each failed attempt. */
  readonly factor?: number;
  /** Upper bound for backoff delay in milliseconds. */
  readonly maxDelayMs?: number;
  /**
   * Jitter strategy applied to retry delays.
   *
   * - `"full"` — randomizes delay between 0 and the computed backoff value
   *   to prevent thundering herd when many clients retry simultaneously.
   * - `"none"` — uses the exact computed backoff value with no randomization.
   *
   * @default "full"
   */
  readonly jitter?: "full" | "none";
  /** Predicate to decide whether a failure should be retried. */
  readonly shouldRetry?: (
    error: TError,
    context: { readonly attempt: number; readonly maxAttempts: number }
  ) => boolean;
  /** Optional AbortSignal to cancel retries. */
  readonly signal?: AbortSignal;
};

const wait = (ms: number): Promise<void> =>
  new Promise((resolve) => {
    setTimeout(resolve, ms);
  });

const checkAborted = (signal?: AbortSignal): void => {
  if (signal?.aborted) {
    throw (signal.reason as Error) ?? new Error("retry aborted");
  }
};

const shouldRetryError = <TError>(
  error: unknown,
  attempt: number,
  maxAttempts: number,
  shouldRetry?: (
    error: TError,
    context: { readonly attempt: number; readonly maxAttempts: number }
  ) => boolean
): boolean => {
  if (attempt >= maxAttempts) {
    return false;
  }
  return shouldRetry?.(error as TError, { attempt, maxAttempts }) ?? true;
};

const computeDelay = (baseDelay: number, jitter: "full" | "none"): number =>
  jitter === "full" ? Math.round(Math.random() * baseDelay) : baseDelay;

/**
 * Executes an async operation with exponential backoff retries.
 *
 * Throws the last observed error when retries are exhausted or when
 * `shouldRetry` returns `false`.
 */
export const withRetry = async <TOutput, TError = unknown>(
  operation: () => Promise<TOutput>,
  options?: RetryOptions<TError>
): Promise<TOutput> => {
  const maxAttempts = Math.max(1, options?.attempts ?? 3);
  const factor = options?.factor ?? 2;
  const maxDelayMs = options?.maxDelayMs ?? 30_000;
  const jitter = options?.jitter ?? "full";

  let attempt = 0;
  let delayMs = options?.delayMs ?? 250;

  for (;;) {
    attempt += 1;
    checkAborted(options?.signal);

    try {
      return await operation();
    } catch (error) {
      if (
        !shouldRetryError(error, attempt, maxAttempts, options?.shouldRetry)
      ) {
        throw error;
      }

      await wait(computeDelay(delayMs, jitter));
      delayMs = Math.min(maxDelayMs, Math.max(1, Math.round(delayMs * factor)));
    }
  }
};
