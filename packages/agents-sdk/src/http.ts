import { Duration, Effect, Either } from "effect";
import { runPromise } from "./effects";
import { StraitAPIError, StraitSDKError } from "./errors";
import type { RetryPolicy, StraitContextOptions } from "./types";

const defaultRetryPolicy: RetryPolicy = {
  maxAttempts: 1,
  baseDelayMs: 25,
  maxDelayMs: 250,
};

function resolveRetryPolicy(
  overrides: Partial<RetryPolicy> | undefined
): RetryPolicy {
  const maxAttempts = overrides?.maxAttempts ?? defaultRetryPolicy.maxAttempts;
  const baseDelayMs = overrides?.baseDelayMs ?? defaultRetryPolicy.baseDelayMs;
  const maxDelayMs = overrides?.maxDelayMs ?? defaultRetryPolicy.maxDelayMs;

  if (!Number.isInteger(maxAttempts) || maxAttempts < 1) {
    throw new StraitSDKError(
      "retry.maxAttempts must be an integer greater than 0"
    );
  }
  if (!Number.isInteger(baseDelayMs) || baseDelayMs < 0) {
    throw new StraitSDKError(
      "retry.baseDelayMs must be a non-negative integer"
    );
  }
  if (!Number.isInteger(maxDelayMs) || maxDelayMs < baseDelayMs) {
    throw new StraitSDKError(
      "retry.maxDelayMs must be an integer greater than or equal to retry.baseDelayMs"
    );
  }

  return { maxAttempts, baseDelayMs, maxDelayMs };
}

function sleepEffect(ms: number): Effect.Effect<void> {
  if (ms <= 0) {
    return Effect.void;
  }
  return Effect.sleep(Duration.millis(ms));
}

function buildSDKURL(baseUrl: string, runId: string, path: string): string {
  const normalizedBaseURL = baseUrl.endsWith("/") ? baseUrl : `${baseUrl}/`;
  return new URL(`sdk/v1/runs/${runId}${path}`, normalizedBaseURL).toString();
}

function isRetryableStatus(status: number): boolean {
  return status === 429 || status >= 500;
}

async function parseResponseBody(response: Response): Promise<unknown> {
  if (response.status === 204) {
    return undefined;
  }

  const bodyText = await response.text();
  if (bodyText.length === 0) {
    return undefined;
  }

  const contentType = response.headers.get("content-type") ?? "";
  if (contentType.includes("json")) {
    try {
      return JSON.parse(bodyText) as unknown;
    } catch {
      return bodyText;
    }
  }

  return bodyText;
}

function makeAPIError(response: Response, body: unknown): StraitAPIError {
  if (body && typeof body === "object") {
    const record = body as Record<string, unknown>;
    const title = typeof record.title === "string" ? record.title : undefined;
    const detail =
      typeof record.detail === "string" ? record.detail : undefined;
    const code = typeof record.code === "string" ? record.code : undefined;
    const details = Array.isArray(record.details)
      ? record.details.filter(
          (item): item is string => typeof item === "string"
        )
      : undefined;

    return new StraitAPIError(
      detail ?? title ?? `request failed with status ${response.status}`,
      {
        status: response.status,
        code,
        details,
        responseBody: body,
      }
    );
  }

  return new StraitAPIError(`request failed with status ${response.status}`, {
    status: response.status,
    responseBody: body,
  });
}

export class StraitHTTPClient {
  readonly #baseUrl: string;
  readonly #runId: string;
  readonly #runToken: string;
  readonly #fetch: typeof fetch;
  readonly #sdkVersion: string;
  readonly #retryPolicy: RetryPolicy;

  constructor(
    options: Pick<
      StraitContextOptions,
      "baseUrl" | "runId" | "runToken" | "fetch" | "sdkVersion" | "retry"
    >
  ) {
    this.#baseUrl = options.baseUrl.trim();
    this.#runId = options.runId.trim();
    this.#runToken = options.runToken.trim();
    this.#fetch = options.fetch ?? fetch;
    this.#sdkVersion = options.sdkVersion?.trim() || "2.0.0";
    this.#retryPolicy = resolveRetryPolicy(options.retry);

    if (this.#baseUrl.length === 0) {
      throw new StraitSDKError("baseUrl is required");
    }
    if (this.#runId.length === 0) {
      throw new StraitSDKError("runId is required");
    }
    if (this.#runToken.length === 0) {
      throw new StraitSDKError("runToken is required");
    }
  }

  post<TResponse>(
    path: string,
    body: unknown,
    options: {
      retryable?: boolean;
      signal?: AbortSignal;
    } = {}
  ): Promise<TResponse> {
    return runPromise(
      this.#requestEffect<TResponse>("POST", path, body, options)
    );
  }

  get<TResponse>(
    path: string,
    options: {
      retryable?: boolean;
      signal?: AbortSignal;
    } = {}
  ): Promise<TResponse> {
    return runPromise(
      this.#requestEffect<TResponse>("GET", path, undefined, options)
    );
  }

  delete<TResponse>(
    path: string,
    options: {
      retryable?: boolean;
      signal?: AbortSignal;
    } = {}
  ): Promise<TResponse> {
    return runPromise(
      this.#requestEffect<TResponse>("DELETE", path, undefined, options)
    );
  }

  #requestEffect<TResponse>(
    method: "DELETE" | "GET" | "POST",
    path: string,
    body: unknown,
    options: {
      retryable?: boolean;
      signal?: AbortSignal;
    }
  ): Effect.Effect<TResponse, StraitAPIError | StraitSDKError | Error> {
    const attempts = options.retryable ? this.#retryPolicy.maxAttempts : 1;
    const url = buildSDKURL(this.#baseUrl, this.#runId, path);

    return Effect.gen(this, function* () {
      let lastError: StraitAPIError | StraitSDKError | Error =
        new StraitSDKError("unreachable retry state");

      for (let attempt = 1; attempt <= attempts; attempt += 1) {
        const result = yield* Effect.either(
          this.#attemptRequest<TResponse>(url, method, body, options.signal)
        );
        if (Either.isRight(result)) {
          return result.right;
        }

        lastError = result.left;
        const hasAttemptsRemaining = attempt < attempts;
        if (!(hasAttemptsRemaining && this.#shouldRetryError(lastError))) {
          return yield* Effect.fail(lastError);
        }

        yield* sleepEffect(
          Math.min(
            this.#retryPolicy.baseDelayMs * attempt,
            this.#retryPolicy.maxDelayMs
          )
        );
      }

      return yield* Effect.fail(lastError);
    });
  }

  #attemptRequest<TResponse>(
    url: string,
    method: "DELETE" | "GET" | "POST",
    body: unknown,
    signal?: AbortSignal
  ): Effect.Effect<TResponse, StraitAPIError | Error> {
    return Effect.tryPromise({
      try: async () => {
        const headers = new Headers({
          Authorization: `Bearer ${this.#runToken}`,
          "X-SDK-Version": this.#sdkVersion,
        });
        let encodedBody: string | undefined;
        if (body !== undefined) {
          headers.set("Content-Type", "application/json");
          encodedBody = JSON.stringify(body);
        }

        const response = await this.#fetch(url, {
          method,
          headers,
          body: encodedBody,
          signal,
        });
        const parsedBody = await parseResponseBody(response);
        if (response.ok) {
          return parsedBody as TResponse;
        }

        throw makeAPIError(response, parsedBody);
      },
      catch: (error) => {
        if (error instanceof StraitAPIError) {
          return error;
        }
        return error instanceof Error ? error : new Error(String(error));
      },
    });
  }

  #shouldRetryError(error: StraitAPIError | Error): boolean {
    if (error instanceof StraitAPIError) {
      return isRetryableStatus(error.status);
    }

    if (error.name === "AbortError") {
      return false;
    }

    return true;
  }
}
