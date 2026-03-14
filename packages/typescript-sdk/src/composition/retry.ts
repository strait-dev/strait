export type RetryOptions<TError = unknown> = {
  readonly attempts?: number;
  readonly delayMs?: number;
  readonly factor?: number;
  readonly maxDelayMs?: number;
  readonly shouldRetry?: (
    error: TError,
    context: { readonly attempt: number; readonly maxAttempts: number }
  ) => boolean;
};

const wait = (ms: number): Promise<void> =>
  new Promise((resolve) => {
    setTimeout(resolve, ms);
  });

export const withRetry = async <TOutput, TError = unknown>(
  operation: () => Promise<TOutput>,
  options?: RetryOptions<TError>
): Promise<TOutput> => {
  const maxAttempts = Math.max(1, options?.attempts ?? 3);
  const factor = options?.factor ?? 2;
  const maxDelayMs = options?.maxDelayMs ?? 30_000;

  let attempt = 0;
  let delayMs = options?.delayMs ?? 250;

  for (;;) {
    attempt += 1;

    try {
      return await operation();
    } catch (error) {
      if (attempt >= maxAttempts) {
        throw error;
      }

      const canRetry =
        options?.shouldRetry?.(error as TError, {
          attempt,
          maxAttempts,
        }) ?? true;

      if (!canRetry) {
        throw error;
      }

      await wait(delayMs);
      delayMs = Math.min(maxDelayMs, Math.max(1, Math.round(delayMs * factor)));
    }
  }
};
