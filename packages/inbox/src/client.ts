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
  InboxFeedReconnectPolicy,
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

type FeedConnectConfig = {
  heartbeatTimeoutMs: number;
  onClose?: () => void;
  onError?: (error: InboxClientError) => void;
  onEvent?: (event: InboxFeedEvent) => void;
  onOpen?: () => void;
  onReconnect?: (
    attempt: number,
    delayMs: number,
    error: InboxClientError
  ) => void;
  reconnect: Required<InboxFeedReconnectPolicy>;
  signal?: AbortSignal;
};

const trailingSlashRegex = /\/+$/;
const newlineRegex = /\r\n/g;
const noop = (): void => undefined;

const defaultFeedReconnectBaseDelayMs = 500;
const defaultFeedReconnectMaxDelayMs = 15_000;
const defaultFeedHeartbeatTimeoutMs = 90_000;

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
    const feedConfig = resolveFeedConnectConfig(input);

    return Effect.tryPromise({
      try: async () => {
        const lifecycleController = new AbortController();
        const cleanupExternalSignal = attachExternalAbort(
          lifecycleController,
          feedConfig.signal
        );

        try {
          const initialReader = await openInboxFeedSession({
            baseUrl,
            config,
            fetchImpl,
            request,
            signal: lifecycleController.signal,
          });
          feedConfig.onOpen?.();

          const readerRef: {
            current: ReadableStreamDefaultReader<Uint8Array> | null;
          } = {
            current: initialReader,
          };

          const closed = runManagedFeedLoop({
            baseUrl,
            config,
            currentReader: initialReader,
            feedConfig,
            fetchImpl,
            lifecycleController,
            readerRef,
            request,
          })
            .catch((cause) => {
              const error = normalizeRequestError(request, cause);
              feedConfig.onError?.(error);
            })
            .finally(() => {
              cleanupExternalSignal();
              feedConfig.onClose?.();
            });

          return {
            close: () => {
              lifecycleController.abort();
              const cancelPromise = readerRef.current?.cancel();
              cancelPromise?.catch(noop);
            },
            closed,
          };
        } catch (cause) {
          cleanupExternalSignal();
          throw cause;
        }
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

const runManagedFeedLoop = async ({
  baseUrl,
  config,
  currentReader,
  feedConfig,
  fetchImpl,
  lifecycleController,
  readerRef,
  request,
}: {
  baseUrl: string;
  config: InboxClientConfig;
  currentReader: ReadableStreamDefaultReader<Uint8Array>;
  feedConfig: FeedConnectConfig;
  fetchImpl: FetchLike;
  lifecycleController: AbortController;
  readerRef: { current: ReadableStreamDefaultReader<Uint8Array> | null };
  request: RequestShape;
}): Promise<void> => {
  let reader = currentReader;
  let reconnectAttempt = 0;

  try {
    while (!lifecycleController.signal.aborted) {
      readerRef.current = reader;
      const sessionError = await runFeedLoop(
        reader,
        request,
        feedConfig,
        lifecycleController.signal
      );
      await reader.cancel().catch(noop);

      if (lifecycleController.signal.aborted) {
        return;
      }

      let reconnectError =
        sessionError ??
        new InboxClientError({
          path: request.path,
          method: request.method,
          details: "feed stream disconnected",
        });

      for (;;) {
        reconnectAttempt += 1;
        if (!canReconnect(reconnectAttempt, feedConfig.reconnect.maxAttempts)) {
          throw reconnectError;
        }

        const delayMs = computeReconnectDelayMs(
          feedConfig.reconnect,
          reconnectAttempt
        );
        feedConfig.onReconnect?.(reconnectAttempt, delayMs, reconnectError);
        feedConfig.onError?.(reconnectError);

        await sleepWithAbort(delayMs, lifecycleController.signal);
        if (lifecycleController.signal.aborted) {
          return;
        }

        try {
          reader = await openInboxFeedSession({
            baseUrl,
            config,
            fetchImpl,
            request,
            signal: lifecycleController.signal,
          });
          feedConfig.onOpen?.();
          reconnectAttempt = 0;
          break;
        } catch (cause) {
          reconnectError = normalizeRequestError(request, cause);
        }
      }
    }
  } finally {
    readerRef.current = null;
  }
};

const canReconnect = (attempt: number, maxAttempts: number): boolean => {
  if (maxAttempts <= 0) {
    return true;
  }
  return attempt <= maxAttempts;
};

const runFeedLoop = async (
  reader: ReadableStreamDefaultReader<Uint8Array>,
  request: RequestShape,
  input: FeedConnectConfig,
  signal: AbortSignal
): Promise<InboxClientError | null> => {
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (!signal.aborted) {
      const chunk = await readFeedChunk(reader, input.heartbeatTimeoutMs);
      if (chunk.done) {
        return null;
      }

      buffer += decoder.decode(chunk.value, { stream: true });
      const split = splitFeedFrames(buffer);
      buffer = split.remainder;

      for (const frame of split.frames) {
        emitFeedFrame(request, frame, input);
      }
    }

    return null;
  } catch (cause) {
    if (signal.aborted) {
      return null;
    }
    return normalizeRequestError(request, cause);
  } finally {
    buffer += decoder.decode();
    const finalSplit = splitFeedFrames(buffer, true);
    for (const frame of finalSplit.frames) {
      emitFeedFrame(request, frame, input);
    }
  }
};

const readFeedChunk = async (
  reader: ReadableStreamDefaultReader<Uint8Array>,
  heartbeatTimeoutMs: number
): Promise<ReadableStreamReadResult<Uint8Array>> => {
  let timeoutID: ReturnType<typeof setTimeout> | undefined;

  try {
    const timeoutPromise = new Promise<never>((_, reject) => {
      timeoutID = setTimeout(() => {
        reject(new Error("feed heartbeat timeout"));
      }, heartbeatTimeoutMs);
    });

    return (await Promise.race([
      reader.read(),
      timeoutPromise,
    ])) as ReadableStreamReadResult<Uint8Array>;
  } finally {
    if (timeoutID !== undefined) {
      clearTimeout(timeoutID);
    }
  }
};

const emitFeedFrame = (
  request: RequestShape,
  frame: string,
  input: FeedConnectConfig
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
  for (;;) {
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
  signal?: AbortSignal
): (() => void) => {
  if (!signal) {
    return noop;
  }

  if (signal.aborted) {
    controller.abort(signal.reason);
    return noop;
  }

  const onAbort = (): void => {
    controller.abort(signal.reason);
  };

  signal.addEventListener("abort", onAbort);
  return () => {
    signal.removeEventListener("abort", onAbort);
  };
};

const openInboxFeedSession = async ({
  baseUrl,
  config,
  fetchImpl,
  request,
  signal,
}: {
  baseUrl: string;
  config: InboxClientConfig;
  fetchImpl: FetchLike;
  request: RequestShape;
  signal: AbortSignal;
}): Promise<ReadableStreamDefaultReader<Uint8Array>> => {
  const token = await resolveToken(config.token);
  if (!token) {
    throw new InboxClientError({
      path: request.path,
      method: request.method,
      details: "missing subscriber token",
    });
  }

  const response = await fetchImpl(buildURL(baseUrl, request.path), {
    method: request.method,
    headers: {
      Accept: "text/event-stream",
      Authorization: `Bearer ${token}`,
      ...(config.headers ?? {}),
    },
    signal,
  });

  if (!response.ok) {
    const text = await response.text();
    throw new InboxClientError({
      path: request.path,
      method: request.method,
      status: response.status,
      details: parseErrorDetails(text),
    });
  }

  if (!response.body) {
    throw new InboxClientError({
      path: request.path,
      method: request.method,
      details: "stream body unavailable",
    });
  }

  return response.body.getReader();
};

const resolveFeedConnectConfig = (
  input: ConnectInboxFeedInput
): FeedConnectConfig => ({
  heartbeatTimeoutMs: normalizePositiveMs(
    input.heartbeatTimeoutMs,
    defaultFeedHeartbeatTimeoutMs
  ),
  onClose: input.onClose,
  onError: input.onError,
  onEvent: input.onEvent,
  onOpen: input.onOpen,
  onReconnect: input.onReconnect,
  reconnect: {
    baseDelayMs: normalizePositiveMs(
      input.reconnect?.baseDelayMs,
      defaultFeedReconnectBaseDelayMs
    ),
    maxAttempts: input.reconnect?.maxAttempts ?? 0,
    maxDelayMs: normalizePositiveMs(
      input.reconnect?.maxDelayMs,
      defaultFeedReconnectMaxDelayMs
    ),
  },
  signal: input.signal,
});

const computeReconnectDelayMs = (
  reconnect: Required<InboxFeedReconnectPolicy>,
  attempt: number
): number => {
  const factor = Math.max(attempt - 1, 0);
  const rawDelay = reconnect.baseDelayMs * 2 ** factor;
  return Math.min(rawDelay, reconnect.maxDelayMs);
};

const normalizePositiveMs = (
  value: number | undefined,
  fallback: number
): number => {
  if (value === undefined || Number.isNaN(value) || value <= 0) {
    return fallback;
  }
  return Math.floor(value);
};

const sleepWithAbort = (delayMs: number, signal: AbortSignal): Promise<void> =>
  new Promise((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }

    const timeoutID = setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, delayMs);

    const onAbort = (): void => {
      clearTimeout(timeoutID);
      signal.removeEventListener("abort", onAbort);
      resolve();
    };

    signal.addEventListener("abort", onAbort);
  });
