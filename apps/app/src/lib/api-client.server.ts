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

type RequestOptions = {
  method?: "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
  body?: unknown;
  params?: Record<string, string | number | boolean | undefined>;
};

/** Make an authenticated request to the Strait Go API. */
export async function apiRequest<T>(
  path: string,
  options: RequestOptions = {}
): Promise<T> {
  const { method = "GET", body, params } = options;
  const base = getApiBaseUrl();
  const url = new URL(path, base);

  if (params) {
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== "") {
        url.searchParams.set(key, String(value));
      }
    }
  }

  // Resolve project ID from the user's activeProjectId
  let projectId: string | undefined;
  try {
    const headers = getRequestHeaders();
    const session = await auth.api.getSession({ headers });
    if (session?.user) {
      const activeProjectId = (session.user as Record<string, unknown>)
        .activeProjectId;
      if (typeof activeProjectId === "string" && activeProjectId) {
        projectId = activeProjectId;
      }
    }
    // Fallback to activeOrganizationId for backwards compatibility
    if (!projectId && session?.session) {
      const activeOrgId = (session.session as Record<string, unknown>)
        .activeOrganizationId;
      if (typeof activeOrgId === "string" && activeOrgId) {
        projectId = activeOrgId;
      }
    }
  } catch {
    // Session resolution is best-effort
  }

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
    let detail = text;
    try {
      const parsed = JSON.parse(text);
      detail = parsed.error || parsed.message || text;
    } catch {
      // raw text is fine
    }
    throw new Error(
      `API ${method} ${path} failed (${response.status}): ${detail}`
    );
  }

  if (response.status === 204) {
    return {} as T;
  }

  return response.json() as Promise<T>;
}
