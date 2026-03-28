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

function sleep(ms: number): Promise<void> {
  if (ms <= 0) {
    return Promise.resolve();
  }
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
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
    return this.#request<TResponse>("POST", path, body, options);
  }

  get<TResponse>(
    path: string,
    options: {
      retryable?: boolean;
      signal?: AbortSignal;
    } = {}
  ): Promise<TResponse> {
    return this.#request<TResponse>("GET", path, undefined, options);
  }

  delete<TResponse>(
    path: string,
    options: {
      retryable?: boolean;
      signal?: AbortSignal;
    } = {}
  ): Promise<TResponse> {
    return this.#request<TResponse>("DELETE", path, undefined, options);
  }

  async #request<TResponse>(
    method: "DELETE" | "GET" | "POST",
    path: string,
    body: unknown,
    options: {
      retryable?: boolean;
      signal?: AbortSignal;
    }
  ): Promise<TResponse> {
    const attempts = options.retryable ? this.#retryPolicy.maxAttempts : 1;
    const url = buildSDKURL(this.#baseUrl, this.#runId, path);

    for (let attempt = 1; attempt <= attempts; attempt += 1) {
      try {
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
          signal: options.signal,
        });

        const parsedBody = await parseResponseBody(response);
        if (response.ok) {
          return parsedBody as TResponse;
        }

        const error = makeAPIError(response, parsedBody);
        if (attempt < attempts && isRetryableStatus(response.status)) {
          await sleep(
            Math.min(
              this.#retryPolicy.baseDelayMs * attempt,
              this.#retryPolicy.maxDelayMs
            )
          );
          continue;
        }
        throw error;
      } catch (error) {
        if (attempt < attempts && error instanceof StraitAPIError === false) {
          await sleep(
            Math.min(
              this.#retryPolicy.baseDelayMs * attempt,
              this.#retryPolicy.maxDelayMs
            )
          );
          continue;
        }
        throw error;
      }
    }

    throw new StraitSDKError("unreachable retry state");
  }
}
