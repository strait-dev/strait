import { Effect, Either } from "effect";
import { domains, type OperationInput, operations } from "./domains/index";
import type { StraitSdkError, ValidationError } from "./errors";
import type {
  HighLevelDomainMap,
  HighLevelFunctionMap,
  HighLevelOperationInput,
  HighLevelResultDomainMap,
  HighLevelResultFunctionMap,
} from "./high-level/generated";
import { buildHighLevelApi } from "./high-level/generated";
import type { GeneratedOperationId } from "./internal/contracts/_generated/contracts";
import type { OperationResponseBodyById } from "./internal/schema/_generated/schema";
import { type FetchLike, provideRuntime } from "./runtime";

type EffectOperation = <ReqBody = unknown, RespBody = unknown>(
  input?: OperationInput<ReqBody, RespBody>
) => Effect.Effect<RespBody, StraitSdkError | ValidationError, never>;

type PromiseOperation = <ReqBody = unknown, RespBody = unknown>(
  input?: OperationInput<ReqBody, RespBody>
) => Promise<RespBody>;

type BaseClient = {
  readonly operations: Readonly<Record<string, EffectOperation>>;
  readonly operationsPromise: Readonly<
    Record<GeneratedOperationId, PromiseOperation>
  >;
  readonly domains: Readonly<Record<string, Record<string, EffectOperation>>>;
  readonly domainsPromise: Readonly<
    Record<string, Record<string, PromiseOperation>>
  >;
  readonly run: <A>(
    effect: Effect.Effect<A, StraitSdkError | ValidationError, never>
  ) => Promise<A>;
};

type HighLevelApiSurface = HighLevelFunctionMap &
  HighLevelDomainMap &
  HighLevelResultFunctionMap &
  HighLevelResultDomainMap & {
    readonly functions: HighLevelFunctionMap;
    readonly namespaces: HighLevelDomainMap & HighLevelResultDomainMap;
    readonly resultFunctions: HighLevelResultFunctionMap;
    readonly resultNamespaces: HighLevelResultDomainMap;
  };

export type StraitClient = BaseClient & HighLevelApiSurface;

const runPromiseUnwrapped = <A, E>(
  effect: Effect.Effect<A, E, never>
): Promise<A> =>
  Effect.runPromise(Effect.either(effect)).then((result) => {
    if (Either.isLeft(result)) {
      return Promise.reject(result.left);
    }

    return result.right;
  });

const bindEffectOperations = (
  input: unknown,
  fetchImpl?: FetchLike
): Readonly<Record<string, EffectOperation>> =>
  Object.fromEntries(
    Object.entries(operations).map(([key, operation]) => [
      key,
      <ReqBody = unknown, RespBody = unknown>(
        operationInput?: OperationInput<ReqBody, RespBody>
      ) =>
        provideRuntime(operation<ReqBody, RespBody>(operationInput), input, {
          fetch: fetchImpl,
        }),
    ])
  ) as Readonly<Record<string, EffectOperation>>;

const bindPromiseOperations = (
  input: unknown,
  fetchImpl?: FetchLike
): Readonly<Record<GeneratedOperationId, PromiseOperation>> =>
  Object.fromEntries(
    Object.entries(operations).map(([key, operation]) => [
      key,
      <ReqBody = unknown, RespBody = unknown>(
        operationInput?: OperationInput<ReqBody, RespBody>
      ) =>
        runPromiseUnwrapped(
          provideRuntime(operation<ReqBody, RespBody>(operationInput), input, {
            fetch: fetchImpl,
          })
        ),
    ])
  ) as Readonly<Record<GeneratedOperationId, PromiseOperation>>;

const bindEffectDomains = (
  input: unknown,
  fetchImpl?: FetchLike
): Readonly<Record<string, Record<string, EffectOperation>>> =>
  Object.fromEntries(
    Object.entries(domains).map(([domainName, operationGroup]) => [
      domainName,
      Object.fromEntries(
        Object.entries(operationGroup).map(([operationName, operation]) => [
          operationName,
          <ReqBody = unknown, RespBody = unknown>(
            operationInput?: OperationInput<ReqBody, RespBody>
          ) =>
            provideRuntime(
              operation<ReqBody, RespBody>(operationInput),
              input,
              {
                fetch: fetchImpl,
              }
            ),
        ])
      ),
    ])
  ) as Readonly<Record<string, Record<string, EffectOperation>>>;

const bindPromiseDomains = (
  input: unknown,
  fetchImpl?: FetchLike
): Readonly<Record<string, Record<string, PromiseOperation>>> =>
  Object.fromEntries(
    Object.entries(domains).map(([domainName, operationGroup]) => [
      domainName,
      Object.fromEntries(
        Object.entries(operationGroup).map(([operationName, operation]) => [
          operationName,
          <ReqBody = unknown, RespBody = unknown>(
            operationInput?: OperationInput<ReqBody, RespBody>
          ) =>
            runPromiseUnwrapped(
              provideRuntime(
                operation<ReqBody, RespBody>(operationInput),
                input,
                {
                  fetch: fetchImpl,
                }
              )
            ),
        ])
      ),
    ])
  ) as Readonly<Record<string, Record<string, PromiseOperation>>>;

export const createClient = (
  input: unknown,
  options?: {
    readonly fetch?: FetchLike;
  }
): StraitClient => {
  const effectOperations = bindEffectOperations(input, options?.fetch);
  const promiseOperations = bindPromiseOperations(input, options?.fetch);

  const effectDomains = bindEffectDomains(input, options?.fetch);
  const promiseDomains = bindPromiseDomains(input, options?.fetch);

  const highLevelApi = buildHighLevelApi(
    <TOperationId extends GeneratedOperationId>(
      operationId: TOperationId,
      operationInput: HighLevelOperationInput<TOperationId>
    ) =>
      promiseOperations[operationId](
        operationInput as OperationInput<unknown, unknown>
      ) as Promise<OperationResponseBodyById[TOperationId]>
  );

  const mergedNamespaces = Object.fromEntries(
    Object.keys({ ...highLevelApi.domains, ...highLevelApi.resultDomains }).map(
      (domainName) => [
        domainName,
        {
          ...(highLevelApi.domains as Record<string, Record<string, unknown>>)[
            domainName
          ],
          ...(
            highLevelApi.resultDomains as Record<
              string,
              Record<string, unknown>
            >
          )[domainName],
        },
      ]
    )
  ) as HighLevelDomainMap & HighLevelResultDomainMap;

  const highLevelSurface = {
    ...highLevelApi.functions,
    ...mergedNamespaces,
    ...highLevelApi.resultFunctions,
    functions: highLevelApi.functions,
    namespaces: mergedNamespaces,
    resultFunctions: highLevelApi.resultFunctions,
    resultNamespaces: highLevelApi.resultDomains,
  } as HighLevelApiSurface;

  return {
    operations: effectOperations,
    operationsPromise: promiseOperations,
    domains: effectDomains,
    domainsPromise: promiseDomains,
    run: <A>(
      effect: Effect.Effect<A, StraitSdkError | ValidationError, never>
    ) => runPromiseUnwrapped(effect),
    ...highLevelSurface,
  };
};

export const createPromiseClient = createClient;
