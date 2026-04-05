import { Effect } from "effect";
import { InboxClientError } from "./errors";
import type {
  FetchLike,
  InboxActionInput,
  InboxActionResponse,
  InboxClient,
  InboxClientConfig,
  InboxItem,
  ListInboxInput,
  NotificationPreference,
  ProcessUnsubscribeRequest,
  ProcessUnsubscribeResponse,
  ResolveUnsubscribeTokenResponse,
  UnreadCountResponse,
  UpdateInboxItemStateInput,
  UpdateNotifyPreferencesRequest,
} from "./types";

type RequestShape = {
  path: string;
  method: "GET" | "POST" | "PATCH" | "PUT";
  query?: Record<string, string | number | undefined>;
  body?: unknown;
};

const trailingSlashRegex = /\/+$/;

export const makeInboxClient = (config: InboxClientConfig): InboxClient => {
  const fetchImpl = resolveFetch(config.fetch);
  const baseUrl = normalizeBaseURL(config.baseUrl);

  const requestJson = <T>(
    request: RequestShape
  ): Effect.Effect<T, InboxClientError> =>
    Effect.tryPromise({
      try: async () => {
        const token = await resolveToken(config.token);
        if (!token) {
          throw new InboxClientError({
            path: request.path,
            method: request.method,
            details: "missing subscriber token",
          });
        }

        const url = buildURL(baseUrl, request.path, request.query);
        const response = await fetchImpl(url, {
          method: request.method,
          headers: {
            Authorization: `Bearer ${token}`,
            "Content-Type": "application/json",
            ...(config.headers ?? {}),
          },
          body:
            request.body === undefined
              ? undefined
              : JSON.stringify(request.body),
        });

        const text = await response.text();
        if (!response.ok) {
          throw new InboxClientError({
            path: request.path,
            method: request.method,
            status: response.status,
            details: parseErrorDetails(text),
          });
        }

        if (response.status === 204 || text.trim() === "") {
          return undefined as T;
        }

        return JSON.parse(text) as T;
      },
      catch: (cause) =>
        cause instanceof InboxClientError
          ? cause
          : new InboxClientError({
              path: request.path,
              method: request.method,
              details: "request failed",
              cause,
            }),
    });

  return {
    listInbox: (input?: ListInboxInput) =>
      requestJson<InboxItem[]>({
        path: "/v1/inbox",
        method: "GET",
        query: {
          limit: input?.limit,
          cursor: input?.cursor,
          state: input?.state,
        },
      }),
    getUnreadCount: () =>
      requestJson<UnreadCountResponse>({
        path: "/v1/inbox/unread-count",
        method: "GET",
      }),
    updateItemState: (input: UpdateInboxItemStateInput) =>
      requestJson<InboxItem>({
        path: `/v1/inbox/${encodeURIComponent(input.itemId)}`,
        method: "PATCH",
        body: { state: input.state },
      }),
    performItemAction: (input: InboxActionInput) =>
      requestJson<InboxActionResponse>({
        path: `/v1/inbox/${encodeURIComponent(input.itemId)}/action`,
        method: "POST",
        body: { action_index: input.actionIndex },
      }),
    markAllRead: () =>
      requestJson<UnreadCountResponse>({
        path: "/v1/inbox/mark-all-read",
        method: "POST",
      }),
    listPreferences: () =>
      requestJson<NotificationPreference[]>({
        path: "/v1/preferences",
        method: "GET",
      }),
    updatePreferences: (input: UpdateNotifyPreferencesRequest) =>
      requestJson<NotificationPreference[]>({
        path: "/v1/preferences",
        method: "PUT",
        body: input,
      }),
    updatePreferencesScope: (
      scope: string,
      input: UpdateNotifyPreferencesRequest
    ) =>
      requestJson<NotificationPreference[]>({
        path: `/v1/preferences/${encodeURIComponent(scope)}`,
        method: "PUT",
        body: input,
      }),
    resolveUnsubscribeToken: (token: string) =>
      requestJson<ResolveUnsubscribeTokenResponse>({
        path: `/v1/unsubscribe/${encodeURIComponent(token)}`,
        method: "GET",
      }),
    processUnsubscribe: (token: string, input?: ProcessUnsubscribeRequest) =>
      requestJson<ProcessUnsubscribeResponse>({
        path: `/v1/unsubscribe/${encodeURIComponent(token)}`,
        method: "POST",
        body: input ?? {},
      }),
    processUnsubscribeOneClick: (token: string) =>
      requestJson<ProcessUnsubscribeResponse>({
        path: `/v1/unsubscribe/${encodeURIComponent(token)}/one-click`,
        method: "POST",
      }),
  };
};

const resolveFetch = (fetchImpl?: FetchLike): FetchLike => {
  if (fetchImpl) {
    return fetchImpl;
  }
  if (typeof globalThis.fetch !== "function") {
    throw new Error("global fetch is not available");
  }
  return globalThis.fetch.bind(globalThis);
};

const normalizeBaseURL = (baseUrl: string): string =>
  baseUrl.replace(trailingSlashRegex, "");

const resolveToken = async (
  token: InboxClientConfig["token"]
): Promise<string> => (typeof token === "function" ? await token() : token);

const buildURL = (
  baseUrl: string,
  path: string,
  query?: Record<string, string | number | undefined>
): string => {
  const url = new URL(path, `${baseUrl}/`);
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== "") {
        url.searchParams.set(key, String(value));
      }
    }
  }
  return url.toString();
};

const parseErrorDetails = (text: string): string => {
  const trimmed = text.trim();
  if (!trimmed) {
    return "request failed";
  }
  try {
    const parsed = JSON.parse(trimmed) as { error?: string; message?: string };
    return parsed.error ?? parsed.message ?? trimmed;
  } catch {
    return trimmed;
  }
};
