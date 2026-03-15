import { Context, Effect, Layer } from "effect";

import { AuthServiceTag } from "./auth-service";
import {
  ConfigServiceTag,
  type ResolveConnectionInput,
} from "./config-service";

/**
 * Generic JSON API request contract.
 */
type JsonQueryValue = string | number | boolean | undefined;

/**
 * Generic JSON API request contract.
 */
export type JsonApiRequest = {
  readonly method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  readonly path: string;
  readonly body?: unknown;
  readonly headers?: Readonly<Record<string, string>>;
  readonly timeoutMs?: number;
  readonly requireAuth?: boolean;
  readonly requireProject?: boolean;
  readonly query?: Readonly<Record<string, JsonQueryValue>>;
  readonly connection?: ResolveConnectionInput;
};

/**
 * Health endpoint response contract used by foundational health checks.
 */
export type HealthResponse = {
  readonly status: string;
};

type ApiService = {
  /** Executes a JSON request and decodes the JSON body as the expected type. */
  readonly requestJson: <TResponse>(
    request: JsonApiRequest
  ) => Effect.Effect<TResponse, Error>;
  /** Calls `/health` against resolved server endpoint. */
  readonly health: (
    connection?: ResolveConnectionInput
  ) => Effect.Effect<HealthResponse, Error>;
};

/**
 * Runtime service that wraps API transport and auth header injection.
 */
export class ApiServiceTag extends Context.Tag("ApiService")<
  ApiServiceTag,
  ApiService
>() {}

const TRAILING_SLASH_RE = /\/+$/;

const normalizePath = (path: string): string =>
  path.startsWith("/") ? path : `/${path}`;

const trimSlash = (value: string): string =>
  value.replace(TRAILING_SLASH_RE, "");

const appendQuery = (path: string, query: URLSearchParams): string => {
  const serialized = query.toString();
  if (serialized.length === 0) {
    return path;
  }

  const delimiter = path.includes("?") ? "&" : "?";
  return `${path}${delimiter}${serialized}`;
};

const buildQuery = (
  request: JsonApiRequest,
  projectId?: string
): URLSearchParams => {
  const query = new URLSearchParams();

  for (const [key, value] of Object.entries(request.query ?? {})) {
    if (value !== undefined) {
      query.set(key, String(value));
    }
  }

  if (request.requireProject === true && projectId) {
    query.set("project_id", projectId);
  }

  return query;
};

const buildTargetUrl = (
  serverUrl: string,
  request: JsonApiRequest,
  projectId?: string
): string => {
  const query = buildQuery(request, projectId);
  const targetPath = appendQuery(normalizePath(request.path), query);
  return `${trimSlash(serverUrl)}${targetPath}`;
};

const readResponseText = (
  response: Response,
  requestLabel: string,
  responseKind: "error response body" | "response body"
): Effect.Effect<string, Error> =>
  Effect.tryPromise({
    try: () => response.text(),
    catch: (error) =>
      new Error(`failed to read ${responseKind} for ${requestLabel}`, {
        cause: error,
      }),
  });

const parseSuccessResponse = <TResponse>(
  response: Response,
  requestLabel: string
): Effect.Effect<TResponse, Error> =>
  Effect.gen(function* () {
    if (response.status === 204) {
      return undefined as TResponse;
    }

    const responseText = yield* readResponseText(
      response,
      requestLabel,
      "response body"
    );
    if (responseText.trim().length === 0) {
      return undefined as TResponse;
    }

    return yield* Effect.try({
      try: () => JSON.parse(responseText) as TResponse,
      catch: (error) =>
        new Error(`failed to parse JSON response for ${requestLabel}`, {
          cause: error,
        }),
    });
  });

const parseErrorResponse = (
  response: Response,
  requestLabel: string
): Effect.Effect<never, Error> =>
  Effect.gen(function* () {
    const responseText = yield* readResponseText(
      response,
      requestLabel,
      "error response body"
    );

    return yield* Effect.fail(
      new Error(
        `request failed with status ${response.status} for ${requestLabel}: ${responseText}`
      )
    );
  });

const makeApiService = Effect.gen(function* () {
  const configService = yield* ConfigServiceTag;
  const authService = yield* AuthServiceTag;

  const service: ApiService = {
    requestJson: <TResponse>(request: JsonApiRequest) =>
      Effect.gen(function* () {
        const connection = yield* configService.resolveConnection({
          ...request.connection,
          requireServer: true,
          requireProject: request.requireProject === true,
        });

        const timeoutMs = request.timeoutMs ?? 30_000;
        const abortController = new AbortController();
        const timeout = setTimeout(() => abortController.abort(), timeoutMs);

        const apiKey =
          request.requireAuth === false
            ? undefined
            : yield* authService.getApiKey(connection.contextName);

        const authorizationHeader =
          apiKey && apiKey.length > 0
            ? { Authorization: `Bearer ${apiKey}` }
            : undefined;

        const targetUrl = buildTargetUrl(
          connection.serverUrl,
          request,
          connection.projectId
        );
        const requestLabel = `${request.method} ${targetUrl}`;

        const response = yield* Effect.tryPromise({
          try: () =>
            fetch(targetUrl, {
              method: request.method,
              headers: {
                "Content-Type": "application/json",
                ...authorizationHeader,
                ...request.headers,
              },
              body:
                request.body === undefined
                  ? undefined
                  : JSON.stringify(request.body),
              signal: abortController.signal,
            }),
          catch: (error) =>
            new Error(`request failed for ${requestLabel}`, {
              cause: error,
            }),
        }).pipe(Effect.ensuring(Effect.sync(() => clearTimeout(timeout))));

        if (!response.ok) {
          return yield* parseErrorResponse(response, requestLabel);
        }

        return yield* parseSuccessResponse<TResponse>(response, requestLabel);
      }),
    health: (connection) =>
      service.requestJson<HealthResponse>({
        method: "GET",
        path: "/health",
        requireAuth: false,
        connection,
      }),
  };

  return service;
});

/**
 * Live API service layer.
 */
export const ApiServiceLive = Layer.effect(ApiServiceTag, makeApiService);
