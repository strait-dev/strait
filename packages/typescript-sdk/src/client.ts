import { Effect, Either } from "effect";
import { domains, type OperationInput, operations } from "./domains/index";
import type { StraitSdkError, ValidationError } from "./errors";
import { type FetchLike, provideRuntime } from "./runtime";

type EffectOperation = <ReqBody = unknown, RespBody = unknown>(
  input?: OperationInput<ReqBody, RespBody>
) => Effect.Effect<RespBody, StraitSdkError | ValidationError, never>;

type PromiseOperation = <ReqBody = unknown, RespBody = unknown>(
  input?: OperationInput<ReqBody, RespBody>
) => Promise<RespBody>;

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
): Readonly<Record<string, PromiseOperation>> =>
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
  ) as Readonly<Record<string, PromiseOperation>>;

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
              { fetch: fetchImpl }
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
                { fetch: fetchImpl }
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
): {
  readonly operations: Readonly<Record<string, EffectOperation>>;
  readonly operationsPromise: Readonly<Record<string, PromiseOperation>>;
  readonly domains: Readonly<Record<string, Record<string, EffectOperation>>>;
  readonly domainsPromise: Readonly<
    Record<string, Record<string, PromiseOperation>>
  >;
  readonly run: <A>(
    effect: Effect.Effect<A, StraitSdkError | ValidationError, never>
  ) => Promise<A>;
} => {
  const effectOperations = bindEffectOperations(input, options?.fetch);
  const promiseOperations = bindPromiseOperations(input, options?.fetch);

  const effectDomains = bindEffectDomains(input, options?.fetch);
  const promiseDomains = bindPromiseDomains(input, options?.fetch);

  return {
    operations: effectOperations,
    operationsPromise: promiseOperations,
    domains: effectDomains,
    domainsPromise: promiseDomains,
    run: <A>(
      effect: Effect.Effect<A, StraitSdkError | ValidationError, never>
    ) => runPromiseUnwrapped(effect),
  };
};
