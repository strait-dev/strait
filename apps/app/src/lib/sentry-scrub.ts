import type * as Sentry from "@sentry/tanstackstart-react";

const FILTERED = "[Filtered]";

const SENSITIVE_QUERY_KEYS = new Set([
  "access_token",
  "auth_token",
  "code",
  "id_token",
  "invitation",
  "invitation_id",
  "refresh_token",
  "state",
  "token",
]);

const SENSITIVE_HEADERS = new Set([
  "authorization",
  "cookie",
  "proxy-authorization",
  "set-cookie",
  "x-api-key",
  "x-internal-secret",
]);

const TOKEN_QUERY_RE =
  /([?&](?:access_token|auth_token|code|id_token|invitation|invitation_id|refresh_token|state|token)=)[^&#\s]+/gi;

function scrubUrl(value: string): string {
  try {
    const url = new URL(value, "https://redacted.invalid");
    for (const key of Array.from(url.searchParams.keys())) {
      if (SENSITIVE_QUERY_KEYS.has(key.toLowerCase())) {
        url.searchParams.set(key, FILTERED);
      }
    }
    if (value.startsWith("http://") || value.startsWith("https://")) {
      return url.toString();
    }
    return `${url.pathname}${url.search}${url.hash}`;
  } catch {
    return value.replace(TOKEN_QUERY_RE, `$1${FILTERED}`);
  }
}

function scrubHeaders(headers: unknown): unknown {
  if (!(headers && typeof headers === "object")) {
    return headers;
  }

  const scrubbed: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(headers)) {
    scrubbed[key] = SENSITIVE_HEADERS.has(key.toLowerCase()) ? FILTERED : value;
  }
  return scrubbed;
}

function scrubString(value: string): string {
  return value.replace(TOKEN_QUERY_RE, `$1${FILTERED}`);
}

export function scrubSentryEvent<E extends Sentry.Event>(event: E): E | null {
  if (event.request?.url) {
    event.request.url = scrubUrl(event.request.url);
  }
  if (event.request?.headers) {
    event.request.headers = scrubHeaders(event.request.headers) as Record<
      string,
      string
    >;
  }
  if (event.user) {
    event.user.email = undefined;
    event.user.ip_address = undefined;
    event.user.username = undefined;
  }
  return event;
}

export function scrubSentryBreadcrumb(
  breadcrumb: Sentry.Breadcrumb
): Sentry.Breadcrumb | null {
  if (breadcrumb.message) {
    breadcrumb.message = scrubString(breadcrumb.message);
  }
  if (breadcrumb.data) {
    const data = { ...breadcrumb.data };
    for (const key of ["url", "to", "from", "href"]) {
      const value = data[key];
      if (typeof value === "string") {
        data[key] = scrubUrl(value);
      }
    }
    if (data.headers) {
      data.headers = scrubHeaders(data.headers);
    }
    breadcrumb.data = data;
  }
  return breadcrumb;
}
