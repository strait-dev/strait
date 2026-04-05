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
  ConnectInboxFeedInput,
  FetchLike,
  InboxActionInput,
  InboxClient,
  InboxClientConfig,
  InboxFeedEvent,
  ListInboxInput,
  ProcessUnsubscribeRequest,
  UpdateInboxItemStateInput,
  UpdateNotifyPreferencesRequest,
} from "./types";

type RequestShape = {
  body?: unknown;
  method: "GET" | "POST" | "PATCH" | "PUT";
  path: string;
  query?: Record<string, string | number | undefined>;
};

const trailingSlashRegex = /\/+$/;
const newlineRegex = /\r\n/g;
const noop = (): void => undefined;

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
      catch: (cause) => normalizeRequestError(request, cause),
    });

  const connectFeed = (
    input: ConnectInboxFeedInput = {}
  ): Effect.Effect<import("./types").InboxFeedConnection, InboxClientError> => {
    const request: RequestShape = {
      method: "GET",
      path: "/v1/inbox/feed",
    };

    return Effect.tryPromise({
      try: async () => {
        const token = await resolveToken(config.token);
        if (!token) {
          throw new InboxClientError({
            path: request.path,
            method: request.method,
            details: "missing subscriber token",
          });
        }

        const abortController = new AbortController();
        const cleanupExternalSignal = attachExternalAbort(
          abortController,
          input.signal
        );

        const response = await fetchImpl(buildURL(baseUrl, request.path), {
          method: request.method,
          headers: {
            Accept: "text/event-stream",
            Authorization: `Bearer ${token}`,
            ...(config.headers ?? {}),
          },
          signal: abortController.signal,
        });

        if (!response.ok) {
          const text = await response.text();
          cleanupExternalSignal();
          throw new InboxClientError({
            path: request.path,
            method: request.method,
            status: response.status,
            details: parseErrorDetails(text),
          });
        }

        if (!response.body) {
          cleanupExternalSignal();
          throw new InboxClientError({
            path: request.path,
            method: request.method,
            details: "stream body unavailable",
          });
        }

        const reader = response.body.getReader();
        input.onOpen?.();

        const closed = runFeedLoop(reader, request, input)
          .catch((cause) => {
            const error = normalizeRequestError(request, cause);
            input.onError?.(error);
          })
          .finally(() => {
            cleanupExternalSignal();
            input.onClose?.();
          });

        return {
          close: () => {
            abortController.abort();
            const cancelPromise = reader.cancel();
            cancelPromise.catch(() => undefined);
          },
          closed,
        };
      },
      catch: (cause) => normalizeRequestError(request, cause),
    });
  };

  return {
    connectFeed,
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

const runFeedLoop = async (
  reader: ReadableStreamDefaultReader<Uint8Array>,
  request: RequestShape,
  input: ConnectInboxFeedInput
): Promise<void> => {
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const chunk = await reader.read();
    if (chunk.done) {
      break;
    }

    buffer += decoder.decode(chunk.value, { stream: true });
    const split = splitFeedFrames(buffer);
    buffer = split.remainder;

    for (const frame of split.frames) {
      emitFeedFrame(request, frame, input);
    }
  }

  buffer += decoder.decode();
  const finalSplit = splitFeedFrames(buffer, true);
  for (const frame of finalSplit.frames) {
    emitFeedFrame(request, frame, input);
  }
};

const emitFeedFrame = (
  request: RequestShape,
  frame: string,
  input: ConnectInboxFeedInput
): void => {
  const parsed = parseFeedFrame(frame);
  if (parsed == null) {
    return;
  }

  try {
    input.onEvent?.(parsed);
  } catch (cause) {
    input.onError?.(
      new InboxClientError({
        path: request.path,
        method: request.method,
        details: "feed event handler failed",
        cause,
      })
    );
  }
};

const splitFeedFrames = (
  source: string,
  flushRemainder = false
): { frames: string[]; remainder: string } => {
  const normalized = source.replace(newlineRegex, "\n");
  const frames: string[] = [];

  let start = 0;
  while (true) {
    const separatorIndex = normalized.indexOf("\n\n", start);
    if (separatorIndex < 0) {
      break;
    }

    frames.push(normalized.slice(start, separatorIndex));
    start = separatorIndex + 2;
  }

  const remainder = normalized.slice(start);
  if (flushRemainder && remainder.trim() !== "") {
    frames.push(remainder);
    return { frames, remainder: "" };
  }

  return { frames, remainder };
};

const parseFeedFrame = (frame: string): InboxFeedEvent | null => {
  const lines = frame.split("\n");
  let eventName = "message";
  const dataLines: string[] = [];

  for (const rawLine of lines) {
    const line = rawLine.trimEnd();
    if (line === "" || line.startsWith(":")) {
      continue;
    }
    if (line.startsWith("event:")) {
      eventName = line.slice("event:".length).trim();
      continue;
    }
    if (line.startsWith("data:")) {
      dataLines.push(line.slice("data:".length).trimStart());
    }
  }

  if (dataLines.length === 0) {
    return null;
  }

  const dataText = dataLines.join("\n");
  return {
    event: eventName,
    data: parseSSEData(dataText),
  };
};

const parseSSEData = (payload: string): unknown => {
  try {
    return JSON.parse(payload) as unknown;
  } catch {
    return payload;
  }
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

const normalizeRequestError = (
  request: RequestShape,
  cause: unknown
): InboxClientError =>
  cause instanceof InboxClientError
    ? cause
    : new InboxClientError({
        path: request.path,
        method: request.method,
        details: "request failed",
        cause,
      });

const attachExternalAbort = (
  controller: AbortController,
  external?: AbortSignal
): (() => void) => {
  if (!external) {
    return noop;
  }

  if (external.aborted) {
    controller.abort(external.reason);
    return noop;
  }

  const onAbort = () => controller.abort(external.reason);
  external.addEventListener("abort", onAbort, { once: true });
  return () => external.removeEventListener("abort", onAbort);
};
