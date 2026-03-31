/**
 * Model fallback middleware for the Vercel AI SDK.
 *
 * When a provider returns a retriable error (rate limit, timeout, unavailable),
 * the middleware catches the error and retries with the next model in the
 * fallback chain. Non-retriable errors (auth, content policy) are not retried.
 */

export type FallbackCondition = "rate_limit" | "timeout" | "unavailable";

export interface ModelFallbackOptions {
  /** Ordered list of fallback model IDs to try after the primary fails. */
  fallbacks: string[];
  /** Called when a fallback is attempted. */
  onFallback?: (fromModel: string, toModel: string, error: unknown) => void;
  /** Error conditions that trigger a fallback. Defaults to all three. */
  retryOn?: FallbackCondition[];
}

const DEFAULT_RETRY_CONDITIONS: FallbackCondition[] = [
  "rate_limit",
  "timeout",
  "unavailable",
];

function isRetriableError(
  error: unknown,
  conditions: FallbackCondition[]
): boolean {
  if (error == null) {
    return false;
  }

  const message =
    error instanceof Error
      ? error.message.toLowerCase()
      : String(error).toLowerCase();
  const status =
    error instanceof Error && "status" in error
      ? (error as { status?: number }).status
      : undefined;

  for (const condition of conditions) {
    switch (condition) {
      case "rate_limit":
        if (status === 429 || message.includes("rate limit")) {
          return true;
        }
        break;
      case "timeout":
        if (
          message.includes("timeout") ||
          message.includes("timed out") ||
          message.includes("deadline exceeded")
        ) {
          return true;
        }
        break;
      case "unavailable":
        if (
          status === 503 ||
          status === 502 ||
          message.includes("unavailable") ||
          message.includes("overloaded")
        ) {
          return true;
        }
        break;
      default:
        break;
    }
  }

  return false;
}

/**
 * Creates a model fallback wrapper for generate/stream calls.
 * This is not a Vercel AI LanguageModelMiddleware -- it wraps the
 * outer generateText/streamText call to retry with different models.
 */
export function withModelFallback<T>(
  fn: (model: string) => Promise<T>,
  primaryModel: string,
  options: ModelFallbackOptions
): Promise<T> {
  const conditions = options.retryOn ?? DEFAULT_RETRY_CONDITIONS;
  const models = [primaryModel, ...options.fallbacks];

  async function attempt(index: number): Promise<T> {
    const model = models[index];
    if (model === undefined) {
      // All models exhausted -- this shouldn't happen, but be safe.
      throw new Error("all fallback models exhausted");
    }
    try {
      return await fn(model);
    } catch (error) {
      const nextIndex = index + 1;
      if (nextIndex < models.length && isRetriableError(error, conditions)) {
        const nextModel = models[nextIndex];
        if (nextModel !== undefined) {
          options.onFallback?.(model, nextModel, error);
        }
        return attempt(nextIndex);
      }
      throw error;
    }
  }

  return attempt(0);
}

export { isRetriableError };
