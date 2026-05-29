import { getRequestHeaders } from "@tanstack/react-start/server";
import { Data, Effect } from "effect";
import { getAuth } from "@/lib/auth.server";

const DEFAULT_API_BASE_URL = "http://localhost:8080";
const DEFAULT_METHOD = "GET";
const DEFAULT_RESPONSE_TYPE = "json";
const INTERNAL_SECRET_HEADER = "X-Internal-Secret";
const PROJECT_ID_HEADER = "X-Project-Id";
const JSON_CONTENT_TYPE = "application/json";

function getApiBaseUrl(): string {
  return process.env.STRAIT_API_URL || DEFAULT_API_BASE_URL;
}

type RequestMethod = "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
type ResponseType = "json" | "text" | "arraybuffer";

export type ApiClientErrorReason =
  | "missing_secret"
  | "invalid_path"
  | "session_resolution_error"
  | "network_error"
  | "http_error"
  | "response_parse_error";

/** Structured failure returned by Effect-based Go API client calls. */
export class ApiClientError extends Data.TaggedError("ApiClientError")<{
  readonly reason: ApiClientErrorReason;
  readonly message: string;
  readonly path: string;
  readonly method: RequestMethod;
  readonly status?: number;
  readonly detail?: string;
  readonly cause?: unknown;
}> {}

/** Converts a typed client failure into the legacy `Error` shape thrown by `apiRequest`. */
export function apiClientErrorToError(error: ApiClientError): Error {
  return new Error(error.message, { cause: error });
}

function getInternalSecretEffect(
  path: string,
  method: RequestMethod
): Effect.Effect<string, ApiClientError> {
  const secret = process.env.INTERNAL_SECRET;
  if (!secret) {
    return Effect.fail(
      new ApiClientError({
        reason: "missing_secret",
        message: "INTERNAL_SECRET is not configured",
        path,
        method,
      })
    );
  }
  return Effect.succeed(secret);
}

/** Options for `apiRequest` calls to the Strait Go API. */
export type RequestOptions = {
  /** HTTP verb used for the Go API request. Defaults to `GET`. */
  method?: RequestMethod;
  /** JSON-serializable request body. Omitted when undefined or nullish. */
  body?: unknown;
  /** Query string values appended after path validation. Empty strings are skipped. */
  params?: Record<string, string | number | boolean | undefined>;
  /** Project ID sent as `X-Project-Id`; `undefined` resolves from session, `null` suppresses it. */
  projectId?: string | null;
  /** Response parser to use after a successful status. Defaults to JSON. */
  responseType?: ResponseType;
};

const encodedRouteControlPattern = /%(?:2e|2f|5c)/i;
const routeControlCharPattern = /[/?#\\]/;

function validateRawApiPath(path: string): void {
  if (!path.startsWith("/")) {
    throw new Error("API path must be absolute");
  }
  if (path.startsWith("//")) {
    throw new Error("API path cannot be protocol-relative");
  }
  if (path.includes("?") || path.includes("#")) {
    throw new Error("API path cannot contain query or fragment syntax");
  }
  if (path.includes("\\")) {
    throw new Error("API path cannot contain backslashes");
  }
  if (encodedRouteControlPattern.test(path)) {
    throw new Error("API path cannot contain encoded route-control bytes");
  }

  for (const segment of path.split("/")) {
    if (segment === "." || segment === "..") {
      throw new Error("API path cannot contain dot segments");
    }
  }
}

export function apiPathSegment(value: string): string {
  if (!value) {
    throw new Error("API path segment cannot be empty");
  }
  if (value === "." || value === "..") {
    throw new Error("API path segment cannot be a dot segment");
  }
  if (routeControlCharPattern.test(value)) {
    throw new Error("API path segment contains route-control characters");
  }
  if (encodedRouteControlPattern.test(value)) {
    throw new Error("API path segment contains encoded route-control bytes");
  }
  return encodeURIComponent(value);
}

export function apiPath(
  strings: TemplateStringsArray,
  ...segments: Array<string | number>
): string {
  let path = strings[0] ?? "";
  for (let i = 0; i < segments.length; i += 1) {
    path += apiPathSegment(String(segments[i])) + (strings[i + 1] ?? "");
  }
  validateRawApiPath(path);
  return path;
}

/** Resolve the active project ID from the current session. */
async function resolveProjectId(): Promise<string | undefined> {
  try {
    const headers = getRequestHeaders();
    const session = await (await getAuth()).api.getSession({ headers });

    if (session?.user) {
      const activeProjectId = (session.user as Record<string, unknown>)
        .activeProjectId;
      if (typeof activeProjectId === "string" && activeProjectId) {
        return activeProjectId;
      }
    }
  } catch {
    // Session resolution is best-effort
  }
  return;
}

/** Build the URL with query params. */
function buildUrl(
  path: string,
  params?: Record<string, string | number | boolean | undefined>
): URL {
  validateRawApiPath(path);
  const url = new URL(path, getApiBaseUrl());
  if (url.pathname !== path) {
    throw new Error("API path was normalized unexpectedly");
  }
  if (params) {
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== "") {
        url.searchParams.set(key, String(value));
      }
    }
  }
  return url;
}

function buildUrlEffect(
  path: string,
  method: RequestMethod,
  params?: Record<string, string | number | boolean | undefined>
): Effect.Effect<URL, ApiClientError> {
  return Effect.try({
    try: () => buildUrl(path, params),
    catch: (cause) =>
      new ApiClientError({
        reason: "invalid_path",
        message: cause instanceof Error ? cause.message : "Invalid API path",
        path,
        method,
        cause,
      }),
  });
}

/** Parse an error response body into a human-readable detail string. */
function parseErrorDetail(text: string): string {
  try {
    const parsed: unknown = JSON.parse(text);
    if (parsed && typeof parsed === "object") {
      const record = parsed as Record<string, unknown>;
      const detail = record.error ?? record.message ?? record.detail;
      if (typeof detail === "string") {
        return detail;
      }
      if (detail !== undefined) {
        return JSON.stringify(detail);
      }
      return JSON.stringify(parsed);
    }
    return typeof parsed === "string" ? parsed : text;
  } catch {
    return text;
  }
}

function resolveProjectIdEffect(
  path: string,
  method: RequestMethod
): Effect.Effect<string | undefined, ApiClientError> {
  return Effect.tryPromise({
    try: resolveProjectId,
    catch: (cause) =>
      new ApiClientError({
        reason: "session_resolution_error",
        message: "Unable to resolve active project",
        path,
        method,
        cause,
      }),
  }).pipe(Effect.catchAll(() => Effect.succeed(undefined)));
}

function responseTextEffect(
  response: Response,
  path: string,
  method: RequestMethod
): Effect.Effect<string, ApiClientError> {
  return Effect.tryPromise({
    try: () => response.text(),
    catch: (cause) =>
      new ApiClientError({
        reason: "response_parse_error",
        message: `API ${method} ${path} error response could not be read`,
        path,
        method,
        cause,
      }),
  });
}

function responseBodyEffect<T>(
  response: Response,
  path: string,
  method: RequestMethod,
  responseType: ResponseType
): Effect.Effect<T, ApiClientError> {
  if (response.status === 204) {
    return Effect.succeed({} as T);
  }

  if (responseType === "text") {
    return Effect.tryPromise({
      try: () => response.text() as Promise<T>,
      catch: (cause) =>
        new ApiClientError({
          reason: "response_parse_error",
          message: `API ${method} ${path} text response could not be read`,
          path,
          method,
          cause,
        }),
    });
  }

  if (responseType === "arraybuffer") {
    return Effect.tryPromise({
      try: () => response.arrayBuffer() as Promise<T>,
      catch: (cause) =>
        new ApiClientError({
          reason: "response_parse_error",
          message: `API ${method} ${path} binary response could not be read`,
          path,
          method,
          cause,
        }),
    });
  }

  return Effect.tryPromise({
    try: () => response.json() as Promise<T>,
    catch: (cause) =>
      new ApiClientError({
        reason: "response_parse_error",
        message: `API ${method} ${path} JSON response could not be parsed`,
        path,
        method,
        cause,
      }),
  });
}

/**
 * Make an authenticated request to the Strait Go API as a typed Effect.
 *
 * This is the primary implementation used by server-side data functions.
 * `apiRequest` is kept as the Promise-compatible adapter for older callers.
 */
export function apiRequestEffect<T>(
  path: string,
  options: RequestOptions = {}
): Effect.Effect<T, ApiClientError> {
  const {
    method = DEFAULT_METHOD,
    body,
    params,
    projectId: projectIdOverride,
    responseType = DEFAULT_RESPONSE_TYPE,
  } = options;

  return Effect.gen(function* () {
    const url = yield* buildUrlEffect(path, method, params);
    const projectId =
      projectIdOverride === undefined
        ? yield* resolveProjectIdEffect(path, method)
        : (projectIdOverride ?? undefined);
    const internalSecret = yield* getInternalSecretEffect(path, method);

    const fetchHeaders: Record<string, string> = {
      [INTERNAL_SECRET_HEADER]: internalSecret,
      "Content-Type": JSON_CONTENT_TYPE,
    };

    if (projectId) {
      fetchHeaders[PROJECT_ID_HEADER] = projectId;
    }

    const response = yield* Effect.tryPromise({
      try: () =>
        fetch(url.toString(), {
          method,
          headers: fetchHeaders,
          body: body ? JSON.stringify(body) : undefined,
        }),
      catch: (cause) =>
        new ApiClientError({
          reason: "network_error",
          message: `API ${method} ${path} request failed`,
          path,
          method,
          cause,
        }),
    });

    if (!response.ok) {
      const text = yield* responseTextEffect(response, path, method);
      const detail = parseErrorDetail(text);
      return yield* Effect.fail(
        new ApiClientError({
          reason: "http_error",
          message: `API ${method} ${path} failed (${response.status}): ${detail}`,
          path,
          method,
          status: response.status,
          detail,
        })
      );
    }

    return yield* responseBodyEffect<T>(response, path, method, responseType);
  });
}

/** Make an authenticated request to the Strait Go API. */
export function apiRequest<T>(
  path: string,
  options: RequestOptions = {}
): Promise<T> {
  return Effect.runPromise(
    apiRequestEffect<T>(path, options).pipe(
      Effect.catchAll((error) => Effect.die(apiClientErrorToError(error)))
    )
  );
}
