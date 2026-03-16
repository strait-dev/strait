import type { Schema } from "effect";

import { fromPromise, type SdkResult } from "../composition/result";
import {
  type GeneratedOperationFunctionName,
  type GeneratedOperationId,
  generatedOperations,
} from "../internal/contracts/_generated/contracts";
import type {
  OperationRequestBodyById,
  OperationResponseBodyById,
} from "../internal/schema/_generated/schema";
import type {
  OperationHeaderParamsById,
  OperationPathParamsById,
  OperationQueryParamsById,
} from "../internal/types/_generated/operations";

type GeneratedOperationRecord = (typeof generatedOperations)[number];

type MaybeRequiredField<TKey extends string, TValue> = [TValue] extends [
  undefined,
]
  ? { readonly [K in TKey]?: never }
  : { readonly [K in TKey]: TValue };

type MaybeOptionalField<TKey extends string, TValue> = [TValue] extends [
  undefined,
]
  ? { readonly [K in TKey]?: never }
  : { readonly [K in TKey]?: TValue };

/**
 * Input shape for a generated high-level operation.
 *
 * Required fields become mandatory only when the operation expects them.
 */
export type HighLevelOperationInput<TOperationId extends GeneratedOperationId> =
  MaybeRequiredField<"pathParams", OperationPathParamsById[TOperationId]> &
    MaybeRequiredField<"query", OperationQueryParamsById[TOperationId]> &
    MaybeOptionalField<"headers", OperationHeaderParamsById[TOperationId]> &
    MaybeOptionalField<"body", OperationRequestBodyById[TOperationId]> & {
      /** Accept additional success status codes for this call. */
      readonly successStatus?: readonly number[];
      /** Optional request runtime schema override. */
      readonly requestSchema?: Schema.Schema<
        OperationRequestBodyById[TOperationId]
      >;
      /** Optional response runtime schema override. */
      readonly responseSchema?: Schema.Schema<
        OperationResponseBodyById[TOperationId]
      >;
    };

type OperationIdByFunctionName<
  TFunctionName extends GeneratedOperationFunctionName,
> = Extract<
  GeneratedOperationRecord,
  { readonly functionName: TFunctionName }
>["id"];

type GeneratedDomainName = GeneratedOperationRecord["domainName"];

type DomainOperationRecord<TDomain extends GeneratedDomainName> = Extract<
  GeneratedOperationRecord,
  { readonly domainName: TDomain }
>;

/**
 * Top-level generated Promise API grouped by operation function name.
 */
export type HighLevelFunctionMap = {
  readonly [TFunctionName in GeneratedOperationFunctionName]: (
    input: HighLevelOperationInput<OperationIdByFunctionName<TFunctionName>>
  ) => Promise<
    OperationResponseBodyById[OperationIdByFunctionName<TFunctionName>]
  >;
};

/**
 * Namespaced generated Promise API grouped by operation domain/tag.
 */
export type HighLevelDomainMap = {
  readonly [TDomain in GeneratedDomainName]: {
    readonly [TOperation in DomainOperationRecord<TDomain> as TOperation["domainMethodName"]]: (
      input: HighLevelOperationInput<TOperation["id"]>
    ) => Promise<OperationResponseBodyById[TOperation["id"]]>;
  };
};

/**
 * Top-level generated Result API for non-GET operations.
 */
export type HighLevelResultFunctionMap = {
  readonly [TOperation in GeneratedOperationRecord as TOperation["method"] extends "GET"
    ? never
    : `${TOperation["functionName"]}Result`]: (
    input: HighLevelOperationInput<TOperation["id"]>
  ) => Promise<SdkResult<OperationResponseBodyById[TOperation["id"]], unknown>>;
};

/**
 * Namespaced generated Result API for non-GET operations.
 */
export type HighLevelResultDomainMap = {
  readonly [TDomain in GeneratedDomainName]: {
    readonly [TOperation in DomainOperationRecord<TDomain> as TOperation["method"] extends "GET"
      ? never
      : `${TOperation["domainMethodName"]}Result`]: (
      input: HighLevelOperationInput<TOperation["id"]>
    ) => Promise<
      SdkResult<OperationResponseBodyById[TOperation["id"]], unknown>
    >;
  };
};

/**
 * Dispatcher used by high-level API builder to execute by operation id.
 */
export type HighLevelExecutor = <TOperationId extends GeneratedOperationId>(
  operationId: TOperationId,
  input: HighLevelOperationInput<TOperationId>
) => Promise<OperationResponseBodyById[TOperationId]>;

/**
 * Builds top-level and namespaced Promise/Result operation maps from generated
 * operation metadata.
 */
export const buildHighLevelApi = (
  execute: HighLevelExecutor
): {
  readonly functions: HighLevelFunctionMap;
  readonly domains: HighLevelDomainMap;
  readonly resultFunctions: HighLevelResultFunctionMap;
  readonly resultDomains: HighLevelResultDomainMap;
} => {
  const functions: Record<string, unknown> = {};
  const domains: Record<string, Record<string, unknown>> = {};
  const resultFunctions: Record<string, unknown> = {};
  const resultDomains: Record<string, Record<string, unknown>> = {};

  for (const operation of generatedOperations) {
    const invoke = (input: unknown) =>
      execute(operation.id as GeneratedOperationId, input as never);

    functions[operation.functionName] = invoke;

    const currentDomain = domains[operation.domainName] ?? {};
    currentDomain[operation.domainMethodName] = invoke;
    domains[operation.domainName] = currentDomain;

    if (operation.method !== "GET") {
      const invokeResult = (input: unknown) => fromPromise(() => invoke(input));
      resultFunctions[`${operation.functionName}Result`] = invokeResult;

      const currentResultDomain = resultDomains[operation.domainName] ?? {};
      currentResultDomain[`${operation.domainMethodName}Result`] = invokeResult;
      resultDomains[operation.domainName] = currentResultDomain;
    }
  }

  return {
    functions: functions as HighLevelFunctionMap,
    domains: domains as HighLevelDomainMap,
    resultFunctions: resultFunctions as HighLevelResultFunctionMap,
    resultDomains: resultDomains as HighLevelResultDomainMap,
  };
};
