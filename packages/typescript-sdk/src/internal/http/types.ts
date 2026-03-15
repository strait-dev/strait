import type { Schema } from "effect";

export type HttpMethod = "DELETE" | "GET" | "PATCH" | "POST" | "PUT";

export type QueryPrimitive = boolean | number | string;

export type HttpRequestOptions<ReqBody = unknown, RespBody = unknown> = {
  readonly method: HttpMethod;
  readonly path: string;
  readonly query?: Readonly<Record<string, QueryPrimitive | null | undefined>>;
  readonly headers?: Readonly<Record<string, string>>;
  readonly body?: ReqBody;
  readonly successStatus?: readonly number[];
  readonly requestSchema?: Schema.Schema<ReqBody>;
  readonly responseSchema?: Schema.Schema<RespBody>;
  /** Optional AbortSignal to cancel the request. Combined with timeout signal. */
  readonly signal?: AbortSignal;
};
