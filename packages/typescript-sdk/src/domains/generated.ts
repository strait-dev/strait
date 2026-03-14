import { Effect, type Schema } from "effect";

import { type StraitSdkError, ValidationError } from "../errors";
import {
  type GeneratedOperation,
  generatedOperationMap,
  generatedOperations,
  generatedOperationsByTag,
} from "../internal/contracts/_generated/contracts";
import { request } from "../internal/http/client";
import { generatedOperationSchemas } from "../internal/schema/_generated/schema";
import type { StraitRuntimeTag } from "../runtime";

export type PathParams = Readonly<Record<string, number | string>>;
export type QueryParams = Readonly<
  Record<string, boolean | number | string | null | undefined>
>;

export type OperationInput<ReqBody = unknown, RespBody = unknown> = {
  readonly pathParams?: PathParams;
  readonly query?: QueryParams;
  readonly headers?: Readonly<Record<string, string>>;
  readonly body?: ReqBody;
  readonly successStatus?: readonly number[];
  readonly requestSchema?: Schema.Schema<ReqBody>;
  readonly responseSchema?: Schema.Schema<RespBody>;
};

export type OperationEffect<RespBody> = Effect.Effect<
  RespBody,
  StraitSdkError,
  StraitRuntimeTag
>;

const pathTokenRegex = /\{([^}]+)\}/g;
const whitespacePattern = /\s+/;

const resolvePath = (
  template: string,
  pathParams?: PathParams
): Effect.Effect<string, ValidationError, never> =>
  Effect.try({
    try: () =>
      template.replace(pathTokenRegex, (_, token: string) => {
        const value = pathParams?.[token];

        if (value === undefined) {
          throw new ValidationError({
            message: `missing path parameter: ${token}`,
          });
        }

        return encodeURIComponent(String(value));
      }),
    catch: (cause) => {
      if (cause instanceof ValidationError) {
        return cause;
      }

      return new ValidationError({
        message: "failed to resolve path parameters",
        issues: [String(cause)],
      });
    },
  });

const invokeOperation = <ReqBody = unknown, RespBody = unknown>(
  operation: GeneratedOperation,
  input?: OperationInput<ReqBody, RespBody>
): OperationEffect<RespBody> =>
  Effect.flatMap(
    resolvePath(operation.path, input?.pathParams),
    (resolvedPath) => {
      const generatedSchemas =
        generatedOperationSchemas[
          operation.id as keyof typeof generatedOperationSchemas
        ];

      const generatedRequestSchema =
        generatedSchemas && "request" in generatedSchemas
          ? (generatedSchemas.request as Schema.Schema<ReqBody>)
          : undefined;
      const generatedResponseSchema =
        generatedSchemas && "response" in generatedSchemas
          ? (generatedSchemas.response as Schema.Schema<RespBody>)
          : undefined;

      return request<ReqBody, RespBody>({
        method: operation.method,
        path: resolvedPath,
        query: input?.query,
        headers: input?.headers,
        body: input?.body,
        successStatus: input?.successStatus,
        requestSchema: input?.requestSchema ?? generatedRequestSchema,
        responseSchema: input?.responseSchema ?? generatedResponseSchema,
      });
    }
  );

export const operations = Object.fromEntries(
  generatedOperations.map((operation) => [
    operation.id,
    <ReqBody = unknown, RespBody = unknown>(
      input?: OperationInput<ReqBody, RespBody>
    ) => invokeOperation<ReqBody, RespBody>(operation, input),
  ])
) as Readonly<
  Record<
    string,
    <ReqBody = unknown, RespBody = unknown>(
      input?: OperationInput<ReqBody, RespBody>
    ) => OperationEffect<RespBody>
  >
>;

const toDomainName = (tag: string): string => {
  const [head, ...tail] = tag
    .replaceAll(/[^a-zA-Z0-9]+/g, " ")
    .trim()
    .split(whitespacePattern)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1));

  return `${head?.charAt(0).toLowerCase() ?? ""}${head?.slice(1) ?? ""}${tail.join("")}`;
};

export const domains = Object.fromEntries(
  Object.entries(generatedOperationsByTag).map(([tag, items]) => {
    const entries = items.map((operation) => [
      operation.id,
      <ReqBody = unknown, RespBody = unknown>(
        input?: OperationInput<ReqBody, RespBody>
      ) => invokeOperation<ReqBody, RespBody>(operation, input),
    ]);

    return [toDomainName(tag), Object.fromEntries(entries)];
  })
) as Readonly<
  Record<
    string,
    Record<
      string,
      <ReqBody = unknown, RespBody = unknown>(
        input?: OperationInput<ReqBody, RespBody>
      ) => OperationEffect<RespBody>
    >
  >
>;

export const getOperation = (
  operationId: string
): GeneratedOperation | undefined =>
  operationId in generatedOperationMap
    ? generatedOperationMap[operationId as keyof typeof generatedOperationMap]
    : undefined;
