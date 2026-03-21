import { getRequestHeaders } from "@tanstack/react-start/server";
import { auth } from "@/lib/auth.server";

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
  responseType?: "json" | "text" | "arraybuffer";
};

/** Resolve the active project ID from the current session. */
async function resolveProjectId(): Promise<string | undefined> {
  try {
    const headers = getRequestHeaders();
    const session = await auth.api.getSession({ headers });

    if (session?.user) {
      const activeProjectId = (session.user as Record<string, unknown>)
        .activeProjectId;
      if (typeof activeProjectId === "string" && activeProjectId) {
        return activeProjectId;
      }
    }

    // Fallback to activeOrganizationId for backwards compatibility
    if (session?.session) {
      const activeOrgId = (session.session as Record<string, unknown>)
        .activeOrganizationId;
      if (typeof activeOrgId === "string" && activeOrgId) {
        return activeOrgId;
      }
    }
  } catch {
    // Session resolution is best-effort
  }
  return undefined;
}

/** Build the URL with query params. */
function buildUrl(
  path: string,
  params?: Record<string, string | number | boolean | undefined>
): URL {
  const url = new URL(path, getApiBaseUrl());
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
  const { method = "GET", body, params, responseType = "json" } = options;
  const url = buildUrl(path, params);
  const projectId = await resolveProjectId();

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
