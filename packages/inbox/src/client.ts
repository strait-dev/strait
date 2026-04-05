import { Effect, Either, ParseResult, Schema } from "effect";
import { InboxClientError } from "./errors";
import {
  InboxActionResponseSchema,
  InboxItemListSchema,
  InboxItemSchema,
  NotificationPreferenceListSchema,
  ProcessUnsubscribeResponseSchema,
  ResolveUnsubscribeTokenResponseSchema,
  UnreadCountResponseSchema,
} from "./schemas";
import type {
  FetchLike,
  InboxActionInput,
  InboxClient,
  InboxClientConfig,
  ListInboxInput,
  ProcessUnsubscribeRequest,
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

  const requestJson = <A, I>(
    request: RequestShape,
    schema: Schema.Schema<A, I>
  ): Effect.Effect<A, InboxClientError> =>
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
          throw new InboxClientError({
            path: request.path,
            method: request.method,
            status: response.status,
            details: "empty response body",
          });
        }

        const parsed = JSON.parse(text) as unknown;
        return decodeWithSchema(request, schema, parsed);
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
      requestJson(
        {
          path: "/v1/inbox",
          method: "GET",
          query: {
            limit: input?.limit,
            cursor: input?.cursor,
            state: input?.state,
          },
        },
        InboxItemListSchema
      ),
    getUnreadCount: () =>
      requestJson(
        {
          path: "/v1/inbox/unread-count",
          method: "GET",
        },
        UnreadCountResponseSchema
      ),
    updateItemState: (input: UpdateInboxItemStateInput) =>
      requestJson(
        {
          path: `/v1/inbox/${encodeURIComponent(input.itemId)}`,
          method: "PATCH",
          body: { state: input.state },
        },
        InboxItemSchema
      ),
    performItemAction: (input: InboxActionInput) =>
      requestJson(
        {
          path: `/v1/inbox/${encodeURIComponent(input.itemId)}/action`,
          method: "POST",
          body: { action_index: input.actionIndex },
        },
        InboxActionResponseSchema
      ),
    markAllRead: () =>
      requestJson(
        {
          path: "/v1/inbox/mark-all-read",
          method: "POST",
        },
        UnreadCountResponseSchema
      ),
    listPreferences: () =>
      requestJson(
        {
          path: "/v1/preferences",
          method: "GET",
        },
        NotificationPreferenceListSchema
      ),
    updatePreferences: (input: UpdateNotifyPreferencesRequest) =>
      requestJson(
        {
          path: "/v1/preferences",
          method: "PUT",
          body: input,
        },
        NotificationPreferenceListSchema
      ),
    updatePreferencesScope: (
      scope: string,
      input: UpdateNotifyPreferencesRequest
    ) =>
      requestJson(
        {
          path: `/v1/preferences/${encodeURIComponent(scope)}`,
          method: "PUT",
          body: input,
        },
        NotificationPreferenceListSchema
      ),
    resolveUnsubscribeToken: (token: string) =>
      requestJson(
        {
          path: `/v1/unsubscribe/${encodeURIComponent(token)}`,
          method: "GET",
        },
        ResolveUnsubscribeTokenResponseSchema
      ),
    processUnsubscribe: (token: string, input?: ProcessUnsubscribeRequest) =>
      requestJson(
        {
          path: `/v1/unsubscribe/${encodeURIComponent(token)}`,
          method: "POST",
          body: input ?? {},
        },
        ProcessUnsubscribeResponseSchema
      ),
    processUnsubscribeOneClick: (token: string) =>
      requestJson(
        {
          path: `/v1/unsubscribe/${encodeURIComponent(token)}/one-click`,
          method: "POST",
        },
        ProcessUnsubscribeResponseSchema
      ),
  };
};

const decodeWithSchema = <A, I>(
  request: RequestShape,
  schema: Schema.Schema<A, I>,
  payload: unknown
): A => {
  const decoded = Schema.decodeUnknownEither(schema)(payload);
  if (Either.isLeft(decoded)) {
    const formatted = ParseResult.TreeFormatter.formatErrorSync(decoded.left);
    throw new InboxClientError({
      path: request.path,
      method: request.method,
      details: `response validation failed: ${formatted}`,
    });
  }
  return decoded.right;
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
