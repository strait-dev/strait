import { getRequestHeaders } from "@tanstack/react-start/server";
import { getAuth } from "@/lib/auth.server";

function getApiBaseUrl(): string {
  return process.env.STRAIT_API_URL || "http://localhost:8080";
}

function getInternalSecret(): string {
  const secret = process.env.INTERNAL_SECRET;
  if (!secret) {
    throw new Error("INTERNAL_SECRET is not configured");
  }
  return secret;
}

/** Options for `apiRequest` calls to the Strait Go API. */
export type RequestOptions = {
  method?: "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
  body?: unknown;
  params?: Record<string, string | number | boolean | undefined>;
  projectId?: string | null;
  responseType?: "json" | "text" | "arraybuffer";
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

/** Parse an error response body into a human-readable detail string. */
function parseErrorDetail(text: string): string {
  try {
    const parsed = JSON.parse(text);
    return parsed.error || parsed.message || text;
  } catch {
    return text;
  }
}

/** Make an authenticated request to the Strait Go API. */
export async function apiRequest<T>(
  path: string,
  options: RequestOptions = {}
): Promise<T> {
  const {
    method = "GET",
    body,
    params,
    projectId: projectIdOverride,
    responseType = "json",
  } = options;
  const url = buildUrl(path, params);
  const projectId =
    projectIdOverride === undefined
      ? await resolveProjectId()
      : (projectIdOverride ?? undefined);

  const fetchHeaders: Record<string, string> = {
    "X-Internal-Secret": getInternalSecret(),
    "Content-Type": "application/json",
  };

  if (projectId) {
    fetchHeaders["X-Project-Id"] = projectId;
  }

  const response = await fetch(url.toString(), {
    method,
    headers: fetchHeaders,
    body: body ? JSON.stringify(body) : undefined,
  });

  if (!response.ok) {
    const text = await response.text();
    const detail = parseErrorDetail(text);
    throw new Error(
      `API ${method} ${path} failed (${response.status}): ${detail}`
    );
  }

  if (response.status === 204) {
    return {} as T;
  }

  if (responseType === "text") {
    return response.text() as Promise<T>;
  }

  if (responseType === "arraybuffer") {
    return response.arrayBuffer() as Promise<T>;
  }

  return response.json() as Promise<T>;
}
