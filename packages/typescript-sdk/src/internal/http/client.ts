import { Effect, Schema } from "effect";

import { getAuthorizationHeader } from "../../config";
import {
  DecodeError,
  mapHttpError,
  type StraitSdkError,
  TransportError,
} from "../../errors";
import { StraitRuntimeTag } from "../../runtime";
import type { HttpRequestOptions } from "./types";

const successStatusesDefault = [200, 201, 202, 204] as const;

const buildUrl = (
  baseUrl: string,
  path: string,
  query?: Readonly<Record<string, boolean | number | string | null | undefined>>
): string => {
  const url = new URL(`${baseUrl}${path}`);

  if (!query) {
    return url.toString();
  }

  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null) {
      continue;
    }

    url.searchParams.set(key, String(value));
  }

  return url.toString();
};

const decodeResponse = <A>(
  schema: Schema.Schema<A> | undefined,
  body: unknown
): Effect.Effect<A, DecodeError> => {
  if (!schema) {
    return Effect.succeed(body as A);
  }

  return Effect.try({
    try: () => Schema.decodeUnknownSync(schema)(body),
    catch: (cause) =>
      new DecodeError({
        message: "response schema validation failed",
        body,
        cause,
      }),
  });
};

const decodeRequestBody = <A>(
  schema: Schema.Schema<A> | undefined,
  body: A
): Effect.Effect<unknown, DecodeError> => {
  if (!schema) {
    return Effect.succeed(body);
  }

  return Effect.try({
    try: () => Schema.encodeSync(schema)(body),
    catch: (cause) =>
      new DecodeError({
        message: "request schema encoding failed",
        body,
        cause,
      }),
  });
};

const readErrorBody = (response: Response): Effect.Effect<unknown, never> =>
  Effect.tryPromise({
    try: async () => {
      const text = await response.text();

      if (text.length === 0) {
        return undefined;
      }

      try {
        return JSON.parse(text) as unknown;
      } catch {
        return text;
      }
    },
    catch: () => undefined,
  }).pipe(Effect.orElseSucceed(() => undefined));

const readSuccessBody = <A>(
  response: Response
): Effect.Effect<A, StraitSdkError> =>
  Effect.tryPromise({
    try: async () => {
      const text = await response.text();
      if (text.length === 0) {
        return undefined as A;
      }

      return JSON.parse(text) as A;
    },
    catch: (cause) =>
      new DecodeError({ message: "failed to decode JSON response", cause }),
  });

const extractErrorMessage = (errorBody: unknown, fallback: string): string => {
  if (typeof errorBody === "object" && errorBody !== null) {
    if (
      "error" in errorBody &&
      typeof (errorBody as Record<string, unknown>).error === "string"
    ) {
      return (errorBody as Record<string, string>).error;
    }
    if (
      "message" in errorBody &&
      typeof (errorBody as Record<string, unknown>).message === "string"
    ) {
      return (errorBody as Record<string, string>).message;
    }
  }

  return fallback || "request failed";
};

export const request = <ReqBody = unknown, RespBody = unknown>(
  options: HttpRequestOptions<ReqBody, RespBody>
): Effect.Effect<RespBody, StraitSdkError, StraitRuntimeTag> =>
  Effect.gen(function* () {
    const runtime = yield* StraitRuntimeTag;

    const url = buildUrl(runtime.config.baseUrl, options.path, options.query);

    const encodedBody =
      options.body === undefined
        ? undefined
        : yield* decodeRequestBody(
            options.requestSchema as Schema.Schema<ReqBody> | undefined,
            options.body
          );

    const response = yield* Effect.tryPromise({
      try: () =>
        runtime.fetch(url, {
          method: options.method,
          headers: {
            Authorization: getAuthorizationHeader(runtime.config.auth),
            "Content-Type": "application/json",
            ...runtime.config.defaultHeaders,
            ...options.headers,
          },
          body:
            encodedBody === undefined ? undefined : JSON.stringify(encodedBody),
          signal: options.signal
            ? AbortSignal.any([
                options.signal,
                AbortSignal.timeout(runtime.config.timeoutMs ?? 30_000),
              ])
            : AbortSignal.timeout(runtime.config.timeoutMs ?? 30_000),
        }),
      catch: (cause) =>
        new TransportError({ message: "request transport error", cause }),
    });

    const successStatuses = options.successStatus ?? successStatusesDefault;

    if (!successStatuses.includes(response.status)) {
      const errorBody = yield* readErrorBody(response);
      const statusMessage = extractErrorMessage(errorBody, response.statusText);

      return yield* Effect.fail(
        mapHttpError(response.status, statusMessage, errorBody)
      );
    }

    const rawBody = yield* readSuccessBody<RespBody>(response);
    return yield* decodeResponse(
      options.responseSchema as Schema.Schema<RespBody> | undefined,
      rawBody
    );
  });
